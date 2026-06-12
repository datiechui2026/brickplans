"""
标签 API

端点：
- GET  /api/tags                            — 全部标签列表
- POST /api/blueprints/{id}/tags            — 给蓝图打标签（批量）
- GET  /api/blueprints/{id}/tags            — 获取蓝图标签
- DELETE /api/blueprints/{id}/tags/{tag_id} — 移除标签关联
"""
from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy import select, delete
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload
from pydantic import BaseModel, Field

from app.api.deps import get_current_user
from app.core.database import get_db
from app.models import Blueprint, Tag, BlueprintTag, User

router = APIRouter(prefix="/api", tags=["tags"])


# ── Schemas ──

class TagBindRequest(BaseModel):
    tags: list[str] = Field(..., min_length=1, max_length=20)


# ── GET /api/tags ──

@router.get("/tags")
async def list_all_tags(db: AsyncSession = Depends(get_db)):
    """获取所有标签（公开接口）"""
    result = await db.execute(select(Tag).order_by(Tag.name))
    tags = result.scalars().all()
    return [{"id": t.id, "name": t.name} for t in tags]


# ── GET /api/blueprints/{blueprint_id}/tags ──

@router.get("/blueprints/{blueprint_id}/tags")
async def get_blueprint_tags(
    blueprint_id: str,
    db: AsyncSession = Depends(get_db),
):
    """获取某蓝图的所有标签"""
    # Check blueprint exists
    bp = await db.get(Blueprint, blueprint_id)
    if not bp:
        raise HTTPException(status_code=404, detail="Blueprint not found")

    result = await db.execute(
        select(Tag)
        .join(BlueprintTag, BlueprintTag.tag_id == Tag.id)
        .where(BlueprintTag.blueprint_id == blueprint_id)
        .order_by(Tag.name)
    )
    tags = result.scalars().all()
    return [{"id": t.id, "name": t.name} for t in tags]


# ── POST /api/blueprints/{blueprint_id}/tags ──

@router.post("/blueprints/{blueprint_id}/tags", status_code=201)
async def bind_tags(
    blueprint_id: str,
    payload: TagBindRequest,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    """给蓝图批量打标签（作者操作）。
    - 不存在的标签名自动创建
    - 已存在的关联不会重复（幂等）
    """
    # Check blueprint
    bp = await db.get(Blueprint, blueprint_id)
    if not bp:
        raise HTTPException(status_code=404, detail="Blueprint not found")
    if bp.author_id != current_user.id:
        raise HTTPException(status_code=403, detail="Only the blueprint author can manage tags")

    # Normalize: strip whitespace, deduplicate
    tag_names = list(dict.fromkeys(n.strip() for n in payload.tags if n.strip()))

    # Find existing tags
    existing_result = await db.execute(
        select(Tag).where(Tag.name.in_(tag_names))
    )
    existing_tags = {t.name: t for t in existing_result.scalars().all()}

    # Create missing tags
    for name in tag_names:
        if name not in existing_tags:
            tag = Tag(name=name)
            db.add(tag)
            existing_tags[name] = tag

    await db.flush()  # ensure all Tag IDs are generated

    # Find existing blueprint-tag associations
    existing_bt_result = await db.execute(
        select(BlueprintTag).where(
            BlueprintTag.blueprint_id == blueprint_id,
            BlueprintTag.tag_id.in_(t.id for t in existing_tags.values()),
        )
    )
    existing_bt = {(bt.blueprint_id, bt.tag_id) for bt in existing_bt_result.scalars().all()}

    # Create missing associations
    for name in tag_names:
        tag = existing_tags[name]
        if (blueprint_id, tag.id) not in existing_bt:
            bt = BlueprintTag(blueprint_id=blueprint_id, tag_id=tag.id)
            db.add(bt)

    await db.commit()

    # Return all tags for this blueprint after binding
    result = await db.execute(
        select(Tag)
        .join(BlueprintTag, BlueprintTag.tag_id == Tag.id)
        .where(BlueprintTag.blueprint_id == blueprint_id)
        .order_by(Tag.name)
    )
    tags = result.scalars().all()
    return [{"id": t.id, "name": t.name} for t in tags]


# ── DELETE /api/blueprints/{blueprint_id}/tags/{tag_id} ──

@router.delete("/blueprints/{blueprint_id}/tags/{tag_id}", status_code=204)
async def remove_tag(
    blueprint_id: str,
    tag_id: str,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    """移除蓝图上的一个标签关联（不会删除 Tag 本身）"""
    # Check blueprint
    bp = await db.get(Blueprint, blueprint_id)
    if not bp:
        raise HTTPException(status_code=404, detail="Blueprint not found")
    if bp.author_id != current_user.id:
        raise HTTPException(status_code=403, detail="Only the blueprint author can manage tags")

    # Check association exists
    result = await db.execute(
        select(BlueprintTag).where(
            BlueprintTag.blueprint_id == blueprint_id,
            BlueprintTag.tag_id == tag_id,
        )
    )
    bt = result.scalar_one_or_none()
    if not bt:
        raise HTTPException(status_code=404, detail="Tag not found on this blueprint")

    await db.delete(bt)
    await db.commit()
