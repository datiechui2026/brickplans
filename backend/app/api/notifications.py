from datetime import datetime, timezone

from fastapi import APIRouter, Depends, Query
from sqlalchemy import func, select, update
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.api.deps import get_current_user
from app.core.database import get_db
from app.models import Notification, User
from app.schemas import NotificationListOut, NotificationOut

router = APIRouter(prefix="/api/notifications", tags=["notifications"])


@router.get("", response_model=NotificationListOut)
async def list_notifications(
    page: int = Query(default=1, ge=1),
    size: int = Query(default=20, ge=1, le=100),
    current_user: User = Depends(get_current_user),
    db: AsyncSession = Depends(get_db),
):
    base_query = select(Notification).where(Notification.user_id == current_user.id)
    total = (await db.execute(select(func.count()).select_from(base_query.subquery()))).scalar() or 0
    unread_count = (await db.execute(
        select(func.count()).where(
            Notification.user_id == current_user.id,
            Notification.is_read == False,
        )
    )).scalar() or 0

    result = await db.execute(
        base_query
        .options(selectinload(Notification.actor))
        .order_by(Notification.created_at.desc())
        .offset((page - 1) * size)
        .limit(size)
    )
    notifications = result.scalars().all()

    return NotificationListOut(
        items=[_to_notification_out(item) for item in notifications],
        total=total,
        unread_count=unread_count,
        page=page,
        page_size=size,
    )


@router.get("/unread-count")
async def unread_count(
    current_user: User = Depends(get_current_user),
    db: AsyncSession = Depends(get_db),
):
    count = (await db.execute(
        select(func.count()).where(
            Notification.user_id == current_user.id,
            Notification.is_read == False,
        )
    )).scalar() or 0
    return {"unread_count": count}


@router.post("/mark-read")
async def mark_notifications_read(
    current_user: User = Depends(get_current_user),
    db: AsyncSession = Depends(get_db),
):
    await db.execute(
        update(Notification)
        .where(Notification.user_id == current_user.id, Notification.is_read == False)
        .values(is_read=True, read_at=datetime.now(timezone.utc))
    )
    await db.commit()
    return {"detail": "Marked as read"}


def _to_notification_out(notification: Notification) -> dict:
    actor = None
    if notification.actor:
        actor = {
            "id": notification.actor.id,
            "username": notification.actor.username,
            "email": notification.actor.email,
            "avatar_url": notification.actor.avatar_url,
            "bio": notification.actor.bio,
            "is_admin": notification.actor.is_admin,
            "created_at": notification.actor.created_at.isoformat() if notification.actor.created_at else "",
        }
    return {
        "id": notification.id,
        "user_id": notification.user_id,
        "actor_id": notification.actor_id,
        "type": notification.type,
        "blueprint_id": notification.blueprint_id,
        "comment_id": notification.comment_id,
        "payload": notification.payload,
        "is_read": notification.is_read,
        "created_at": notification.created_at.isoformat() if notification.created_at else "",
        "read_at": notification.read_at.isoformat() if notification.read_at else None,
        "actor": actor,
    }
