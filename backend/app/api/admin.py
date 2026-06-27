"""
管理员 API：作品审核、全部作品管理
"""
from typing import Optional

from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy import select, func, or_
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.api.deps import get_current_user, get_current_admin
from app.core.database import get_db
from app.models import User, Blueprint, BlueprintTag, BlueprintImage, Report
from app.schemas import BlueprintOut, BlueprintListOut, AdminReportOut, AdminReportItem, AdminReportListOut

router = APIRouter(prefix="/api/admin", tags=["admin"])


def _to_admin_blueprint_out(bp: Blueprint) -> dict:
    """Full BlueprintOut including author info for admin views."""
    tags = []
    try:
        tags = [bt.tag.name for bt in bp.tags if hasattr(bt, 'tag') and bt.tag]
    except Exception:
        pass

    author = None
    try:
        if bp.author:
            author = {
                "id": bp.author.id,
                "username": bp.author.username,
                "email": bp.author.email,
                "avatar_url": bp.author.avatar_url,
                "bio": bp.author.bio,
                "is_admin": bp.author.is_admin,
                "created_at": bp.author.created_at.isoformat() if bp.author.created_at else "",
            }
    except Exception:
        pass

    images = []
    try:
        for img in (bp.images or []):
            images.append({
                "id": img.id,
                "blueprint_id": img.blueprint_id,
                "url": img.url,
                "object_key": img.object_key,
                "sort_order": img.sort_order,
                "is_cover": img.is_cover,
                "file_type": getattr(img, 'file_type', 'image') or 'image',
            })
    except Exception:
        pass

    cover_url = None
    for img in images:
        if img.get("is_cover") and img.get("file_type", "image") != "pdf":
            cover_url = img["url"]
            break
    if not cover_url:
        for img in images:
            if img.get("file_type", "image") != "pdf":
                cover_url = img["url"]
                break
    if not cover_url and images:
        cover_url = images[0]["url"]

    return {
        "id": bp.id,
        "author_id": bp.author_id,
        "title": bp.title,
        "slug": bp.slug,
        "description": bp.description,
        "difficulty": bp.difficulty,
        "piece_count": bp.piece_count,
        "category": bp.category,
        "dimensions": bp.dimensions,
        "part_list": bp.part_list,
        "view_count": bp.view_count,
        "like_count": bp.like_count,
        "favorite_count": 0,
        "is_liked": False,
        "cover_url": cover_url,
        "is_published": bp.is_published,
        "created_at": bp.created_at.isoformat() if bp.created_at else "",
        "updated_at": bp.updated_at.isoformat() if bp.updated_at else "",
        "author": author,
        "images": images,
        "tags": tags,
    }


@router.get("/blueprints", response_model=BlueprintListOut)
async def admin_list_blueprints(
    page: int = Query(default=1, ge=1),
    size: int = Query(default=20, ge=1, le=100),
    q: Optional[str] = Query(default=None),
    admin: User = Depends(get_current_admin),
    db: AsyncSession = Depends(get_db),
):
    """管理员查看全部作品（含未发布），支持标题/作者搜索。"""
    base_query = select(Blueprint).options(
        selectinload(Blueprint.author),
        selectinload(Blueprint.images),
        selectinload(Blueprint.tags).selectinload(BlueprintTag.tag),
    )

    if q:
        base_query = base_query.outerjoin(Blueprint.author).where(
            or_(
                Blueprint.title.ilike(f"%{q}%"),
                User.username.ilike(f"%{q}%"),
            )
        )

    count_query = select(func.count()).select_from(base_query.subquery())
    total = (await db.execute(count_query)).scalar() or 0

    base_query = base_query.order_by(Blueprint.created_at.desc())
    base_query = base_query.offset((page - 1) * size).limit(size)

    result = await db.execute(base_query)
    blueprints = result.unique().scalars().all()

    return BlueprintListOut(
        items=[_to_admin_blueprint_out(bp) for bp in blueprints],
        total=total,
        page=page,
        page_size=size,
    )


@router.get("/blueprints/pending", response_model=BlueprintListOut)
async def admin_pending_blueprints(
    page: int = Query(default=1, ge=1),
    size: int = Query(default=20, ge=1, le=100),
    admin: User = Depends(get_current_admin),
    db: AsyncSession = Depends(get_db),
):
    """待审核列表（is_published=False）。"""
    base_query = select(Blueprint).options(
        selectinload(Blueprint.author),
        selectinload(Blueprint.images),
        selectinload(Blueprint.tags).selectinload(BlueprintTag.tag),
    ).where(Blueprint.is_published == False)

    count_query = select(func.count()).select_from(base_query.subquery())
    total = (await db.execute(count_query)).scalar() or 0

    base_query = base_query.order_by(Blueprint.created_at.desc())
    base_query = base_query.offset((page - 1) * size).limit(size)

    result = await db.execute(base_query)
    blueprints = result.unique().scalars().all()

    return BlueprintListOut(
        items=[_to_admin_blueprint_out(bp) for bp in blueprints],
        total=total,
        page=page,
        page_size=size,
    )


@router.put("/blueprints/{blueprint_id}/publish")
async def admin_publish(
    blueprint_id: str,
    admin: User = Depends(get_current_admin),
    db: AsyncSession = Depends(get_db),
):
    bp = await db.get(Blueprint, blueprint_id)
    if not bp:
        raise HTTPException(404, "Blueprint not found")
    bp.is_published = True
    await db.commit()
    return {"detail": "Published"}


@router.put("/blueprints/{blueprint_id}/unpublish")
async def admin_unpublish(
    blueprint_id: str,
    admin: User = Depends(get_current_admin),
    db: AsyncSession = Depends(get_db),
):
    bp = await db.get(Blueprint, blueprint_id)
    if not bp:
        raise HTTPException(404, "Blueprint not found")
    bp.is_published = False
    await db.commit()
    return {"detail": "Unpublished"}


@router.delete("/blueprints/{blueprint_id}", status_code=204)
async def admin_delete(
    blueprint_id: str,
    admin: User = Depends(get_current_admin),
    db: AsyncSession = Depends(get_db),
):
    bp = await db.get(Blueprint, blueprint_id)
    if not bp:
        raise HTTPException(404, "Blueprint not found")
    await db.delete(bp)
    await db.commit()


# ────────────────────────── Reports ──────────────────────────

def _to_admin_report_out(report: Report) -> dict:
    """Serialize a Report with reporter info for admin view."""
    reporter = None
    try:
        if report.reporter:
            reporter = {
                "id": report.reporter.id,
                "username": report.reporter.username,
                "email": report.reporter.email,
                "avatar_url": report.reporter.avatar_url,
                "bio": report.reporter.bio,
                "is_admin": report.reporter.is_admin,
                "created_at": report.reporter.created_at.isoformat() if report.reporter.created_at else "",
            }
    except Exception:
        pass

    return {
        "id": report.id,
        "reporter_id": report.reporter_id,
        "blueprint_id": report.blueprint_id,
        "reason": report.reason,
        "detail": report.detail,
        "status": report.status,
        "created_at": report.created_at.isoformat() if report.created_at else "",
        "reporter": reporter,
    }


@router.get("/reports", response_model=AdminReportListOut)
async def admin_list_reports(
    page: int = Query(default=1, ge=1),
    size: int = Query(default=20, ge=1, le=100),
    admin: User = Depends(get_current_admin),
    db: AsyncSession = Depends(get_db),
):
    """管理员查看被举报的图纸列表（按被举报图纸分组）。"""
    # Subquery: count reports per blueprint, get the latest report time
    report_counts = (
        select(
            Report.blueprint_id,
            func.count(Report.id).label("report_count"),
            func.max(Report.created_at).label("latest_report_at"),
        )
        .group_by(Report.blueprint_id)
        .subquery()
    )

    # Count total unique blueprints with reports
    total = (await db.execute(
        select(func.count()).select_from(report_counts)
    )).scalar() or 0

    # Get blueprint IDs for current page, ordered by most recent report
    bp_ids_query = (
        select(report_counts.c.blueprint_id, report_counts.c.report_count)
        .order_by(report_counts.c.latest_report_at.desc())
        .offset((page - 1) * size)
        .limit(size)
    )
    bp_id_result = await db.execute(bp_ids_query)
    bp_rows = bp_id_result.all()

    items = []
    for bp_id, report_count in bp_rows:
        # Load blueprint with author and images
        bp_result = await db.execute(
            select(Blueprint)
            .options(
                selectinload(Blueprint.author),
                selectinload(Blueprint.images),
                selectinload(Blueprint.tags).selectinload(BlueprintTag.tag),
            )
            .where(Blueprint.id == bp_id)
        )
        blueprint = bp_result.scalar_one_or_none()
        if not blueprint:
            continue

        # Load all reports for this blueprint with reporter info
        reports_result = await db.execute(
            select(Report)
            .options(selectinload(Report.reporter))
            .where(Report.blueprint_id == bp_id)
            .order_by(Report.created_at.desc())
        )
        reports = reports_result.unique().scalars().all()

        items.append(AdminReportItem(
            blueprint=_to_admin_blueprint_out(blueprint),
            report_count=report_count,
            reports=[_to_admin_report_out(r) for r in reports],
        ))

    return AdminReportListOut(
        items=items,
        total=total,
        page=page,
        page_size=size,
    )
