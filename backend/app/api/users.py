"""
用户 API：个人主页 — 用户信息、作品集、收藏
"""
from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy import select, func, desc
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.core.database import get_db
from app.models import User, Blueprint, Favorite, BlueprintTag
from app.api.blueprints import _to_blueprint_out, _to_user_out
from app.schemas import BlueprintListOut

router = APIRouter(prefix="/api/users", tags=["users"])


def _to_profile(user: User, bp_count: int, fav_count: int) -> dict:
    data = _to_user_out(user)
    data["blueprint_count"] = bp_count
    data["favorite_count"] = fav_count
    return data


@router.get("/{username}")
async def get_user_profile(
    username: str,
    db: AsyncSession = Depends(get_db),
):
    result = await db.execute(
        select(User).where(User.username == username)
    )
    user = result.scalar_one_or_none()
    if not user:
        raise HTTPException(status_code=404, detail="User not found")

    # Stats
    bp_count_result = await db.execute(
        select(func.count()).where(
            Blueprint.author_id == user.id,
            Blueprint.is_published == True,
        )
    )
    bp_count = bp_count_result.scalar() or 0

    fav_count_result = await db.execute(
        select(func.count()).where(Favorite.user_id == user.id)
    )
    fav_count = fav_count_result.scalar() or 0

    return _to_profile(user, bp_count, fav_count)


@router.get("/{username}/blueprints", response_model=BlueprintListOut)
async def get_user_blueprints(
    username: str,
    page: int = Query(default=1, ge=1),
    size: int = Query(default=12, ge=1, le=50),
    db: AsyncSession = Depends(get_db),
):
    # Find user
    user_result = await db.execute(
        select(User).where(User.username == username)
    )
    user = user_result.scalar_one_or_none()
    if not user:
        raise HTTPException(status_code=404, detail="User not found")

    # Count
    count_q = select(func.count()).where(
        Blueprint.author_id == user.id,
    )
    total = (await db.execute(count_q)).scalar() or 0

    # Query blueprints (include unpublished for the author's own listing)
    offset = (page - 1) * size
    query = (
        select(Blueprint)
        .options(
            selectinload(Blueprint.author),
            selectinload(Blueprint.images),
            selectinload(Blueprint.tags).selectinload(BlueprintTag.tag),
        )
        .where(Blueprint.author_id == user.id)
        .order_by(desc(Blueprint.created_at))
        .offset(offset)
        .limit(size)
    )
    result = await db.execute(query)
    blueprints = result.scalars().all()

    return BlueprintListOut(
        items=[_to_blueprint_out(bp) for bp in blueprints],
        total=total,
        page=page,
        page_size=size,
    )


@router.get("/{username}/favorites", response_model=BlueprintListOut)
async def get_user_favorites(
    username: str,
    page: int = Query(default=1, ge=1),
    size: int = Query(default=12, ge=1, le=50),
    db: AsyncSession = Depends(get_db),
):
    # Find user
    user_result = await db.execute(
        select(User).where(User.username == username)
    )
    user = user_result.scalar_one_or_none()
    if not user:
        raise HTTPException(status_code=404, detail="User not found")

    # Count favorites
    count_q = select(func.count()).where(Favorite.user_id == user.id)
    total = (await db.execute(count_q)).scalar() or 0

    # Query favorited blueprint IDs
    offset = (page - 1) * size
    fav_query = (
        select(Blueprint)
        .options(
            selectinload(Blueprint.author),
            selectinload(Blueprint.images),
            selectinload(Blueprint.tags).selectinload(BlueprintTag.tag),
        )
        .join(Favorite, Favorite.blueprint_id == Blueprint.id)
        .where(Favorite.user_id == user.id)
        .where(Blueprint.is_published == True)
        .order_by(desc(Favorite.created_at))
        .offset(offset)
        .limit(size)
    )
    result = await db.execute(fav_query)
    blueprints = result.scalars().all()

    return BlueprintListOut(
        items=[_to_blueprint_out(bp) for bp in blueprints],
        total=total,
        page=page,
        page_size=size,
    )
