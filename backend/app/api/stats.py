"""
平台统计 API
"""
from fastapi import APIRouter, Depends
from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.database import get_db
from app.models import Blueprint, User, Favorite, Like, Report
from app.schemas import StatsResponse

router = APIRouter(prefix="/api", tags=["stats"])


@router.get("/stats", response_model=StatsResponse)
async def get_stats(db: AsyncSession = Depends(get_db)):
    """返回平台统计数据（公开）。"""
    total_blueprints = (await db.execute(
        select(func.count()).select_from(Blueprint).where(Blueprint.is_published == True)
    )).scalar() or 0

    total_users = (await db.execute(
        select(func.count()).select_from(User)
    )).scalar() or 0

    total_favorites = (await db.execute(
        select(func.count()).select_from(Favorite)
    )).scalar() or 0

    total_pieces = (await db.execute(
        select(func.coalesce(func.sum(Blueprint.piece_count), 0))
        .where(Blueprint.is_published == True)
    )).scalar() or 0

    total_views = (await db.execute(
        select(func.coalesce(func.sum(Blueprint.view_count), 0))
    )).scalar() or 0

    total_likes = (await db.execute(
        select(func.count()).select_from(Like)
    )).scalar() or 0

    pending_count = (await db.execute(
        select(func.count()).select_from(Blueprint).where(Blueprint.is_published == False)
    )).scalar() or 0

    report_count = (await db.execute(
        select(func.count(func.distinct(Report.blueprint_id)))
    )).scalar() or 0

    return StatsResponse(
        total_blueprints=total_blueprints,
        total_users=total_users,
        total_favorites=total_favorites,
        total_pieces=total_pieces,
        total_views=total_views,
        total_likes=total_likes,
        pending_count=pending_count,
        report_count=report_count,
    )
