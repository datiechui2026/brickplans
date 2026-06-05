from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy import select, func, or_
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.core.database import get_db
from app.models import Blueprint, BlueprintImage, BlueprintTag, Tag, Favorite, User
from app.schemas import (
    BlueprintCreate, BlueprintUpdate, BlueprintOut, BlueprintImageOut,
    BlueprintListOut,
)

router = APIRouter(prefix="/api/blueprints", tags=["blueprints"])


async def _get_blueprint_or_404(db: AsyncSession, bp_id: str) -> Blueprint:
    result = await db.execute(
        select(Blueprint)
        .options(
            selectinload(Blueprint.author),
            selectinload(Blueprint.images),
            selectinload(Blueprint.tags).selectinload(BlueprintTag),
        )
        .where(Blueprint.id == bp_id)
    )
    bp = result.scalar_one_or_none()
    if not bp:
        raise HTTPException(status_code=404, detail="Blueprint not found")
    return bp


def _blueprint_to_out(bp: Blueprint) -> BlueprintOut:
    return BlueprintOut(
        id=bp.id,
        author_id=bp.author_id,
        title=bp.title,
        description=bp.description,
        difficulty=bp.difficulty,
        piece_count=bp.piece_count,
        category=bp.category,
        dimensions=bp.dimensions,
        part_list=bp.part_list,
        view_count=bp.view_count,
        is_published=bp.is_published,
        created_at=bp.created_at.isoformat(),
        updated_at=bp.updated_at.isoformat(),
        author={
            "id": bp.author.id,
            "username": bp.author.username,
            "email": bp.author.email,
            "avatar_url": bp.author.avatar_url,
            "bio": bp.author.bio,
            "created_at": bp.author.created_at.isoformat(),
        } if bp.author else None,
        images=[
            BlueprintImageOut(id=img.id, url=img.url, sort_order=img.sort_order, is_cover=img.is_cover)
            for img in (bp.images or [])
        ],
        tags=[bt.tag_id for bt in (bp.tags or [])],
    )


@router.get("", response_model=BlueprintListOut)
async def list_blueprints(
    page: int = Query(1, ge=1),
    page_size: int = Query(20, ge=1, le=100),
    category: str | None = None,
    difficulty: int | None = None,
    tag: str | None = None,
    sort: str = "latest",
    q: str | None = None,
    db: AsyncSession = Depends(get_db),
):
    query = select(Blueprint).options(
        selectinload(Blueprint.author),
        selectinload(Blueprint.images),
        selectinload(Blueprint.tags),
    ).where(Blueprint.is_published == True)

    if category:
        query = query.where(Blueprint.category == category)
    if difficulty:
        query = query.where(Blueprint.difficulty == difficulty)
    if q:
        query = query.where(
            or_(
                Blueprint.title.ilike(f"%{q}%"),
                Blueprint.description.ilike(f"%{q}%"),
            )
        )

    # Count
    count_query = select(func.count()).select_from(query.subquery())
    total = (await db.execute(count_query)).scalar() or 0

    # Sort
    if sort == "popular":
        query = query.order_by(Blueprint.view_count.desc())
    else:
        query = query.order_by(Blueprint.created_at.desc())

    # Paginate
    query = query.offset((page - 1) * page_size).limit(page_size)
    result = await db.execute(query)
    blueprints = result.unique().scalars().all()

    return BlueprintListOut(
        items=[_blueprint_to_out(bp) for bp in blueprints],
        total=total,
        page=page,
        page_size=page_size,
    )


@router.get("/{bp_id}", response_model=BlueprintOut)
async def get_blueprint(bp_id: str, db: AsyncSession = Depends(get_db)):
    bp = await _get_blueprint_or_404(db, bp_id)
    bp.view_count += 1
    await db.flush()
    return _blueprint_to_out(bp)


@router.post("", response_model=BlueprintOut, status_code=201)
async def create_blueprint(payload: BlueprintCreate, db: AsyncSession = Depends(get_db)):
    # TODO: get user from JWT auth
    bp = Blueprint(
        author_id="00000000-0000-0000-0000-000000000001",  # placeholder
        title=payload.title,
        description=payload.description,
        difficulty=payload.difficulty,
        piece_count=payload.piece_count,
        category=payload.category,
        dimensions=payload.dimensions,
        part_list=payload.part_list,
        is_published=payload.is_published,
    )
    db.add(bp)
    await db.flush()

    result = await db.execute(
        select(Blueprint).options(
            selectinload(Blueprint.author),
            selectinload(Blueprint.images),
            selectinload(Blueprint.tags),
        ).where(Blueprint.id == bp.id)
    )
    return _blueprint_to_out(result.scalar_one())


@router.put("/{bp_id}", response_model=BlueprintOut)
async def update_blueprint(bp_id: str, payload: BlueprintUpdate, db: AsyncSession = Depends(get_db)):
    bp = await _get_blueprint_or_404(db, bp_id)
    update_data = payload.model_dump(exclude_unset=True)
    for key, value in update_data.items():
        setattr(bp, key, value)
    await db.flush()

    result = await db.execute(
        select(Blueprint).options(
            selectinload(Blueprint.author),
            selectinload(Blueprint.images),
            selectinload(Blueprint.tags),
        ).where(Blueprint.id == bp.id)
    )
    return _blueprint_to_out(result.scalar_one())


@router.delete("/{bp_id}", status_code=204)
async def delete_blueprint(bp_id: str, db: AsyncSession = Depends(get_db)):
    bp = await _get_blueprint_or_404(db, bp_id)
    await db.delete(bp)
    await db.flush()


@router.post("/{bp_id}/favorite", status_code=200)
async def toggle_favorite(bp_id: str, db: AsyncSession = Depends(get_db)):
    # TODO: get user from JWT auth
    user_id = "00000000-0000-0000-0000-000000000001"
    bp = await _get_blueprint_or_404(db, bp_id)

    existing = await db.execute(
        select(Favorite).where(
            Favorite.user_id == user_id, Favorite.blueprint_id == bp_id
        )
    )
    fav = existing.scalar_one_or_none()

    if fav:
        await db.delete(fav)
        return {"favorited": False}
    else:
        db.add(Favorite(user_id=user_id, blueprint_id=bp_id))
        return {"favorited": True}
