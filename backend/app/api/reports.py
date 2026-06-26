"""
举报 API：创建举报
"""
from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_user
from app.core.database import get_db
from app.models import User, Blueprint, Report
from app.schemas import ReportCreate, ReportOut

router = APIRouter(prefix="/api/reports", tags=["reports"])


@router.post("", response_model=ReportOut, status_code=201)
async def create_report(
    payload: ReportCreate,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    # Validate blueprint exists
    bp_result = await db.execute(select(Blueprint).where(Blueprint.id == payload.blueprint_id))
    blueprint = bp_result.scalar_one_or_none()
    if not blueprint:
        raise HTTPException(status_code=404, detail="Blueprint not found")

    # Check reporter hasn't already reported this blueprint
    existing = await db.execute(
        select(Report).where(
            Report.reporter_id == current_user.id,
            Report.blueprint_id == payload.blueprint_id,
        )
    )
    if existing.scalar_one_or_none():
        raise HTTPException(status_code=409, detail="You have already reported this blueprint")

    # Validate reason enum
    valid_reasons = {"inappropriate", "copyright", "incomplete", "spam", "other"}
    if payload.reason not in valid_reasons:
        raise HTTPException(
            status_code=422,
            detail=f"Invalid reason. Must be one of: {', '.join(valid_reasons)}",
        )

    # Create report
    report = Report(
        reporter_id=current_user.id,
        blueprint_id=payload.blueprint_id,
        reason=payload.reason,
        detail=payload.detail,
    )
    db.add(report)
    await db.flush()

    # Count reports for this blueprint
    count_result = await db.execute(
        select(func.count()).where(Report.blueprint_id == payload.blueprint_id)
    )
    report_count = count_result.scalar() or 0

    # If >= 3 reports, auto-unpublish
    if report_count >= 3:
        blueprint.is_published = False

    await db.commit()
    await db.refresh(report)

    return _to_report_out(report)


def _to_report_out(report: Report) -> dict:
    return {
        "id": report.id,
        "reporter_id": report.reporter_id,
        "blueprint_id": report.blueprint_id,
        "reason": report.reason,
        "detail": report.detail,
        "status": report.status,
        "created_at": report.created_at.isoformat() if report.created_at else "",
    }
