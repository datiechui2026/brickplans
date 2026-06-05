"""
蓝图 API：CRUD + 搜索筛选 + 收藏 + 评论
"""
import re
from typing import Optional

from fastapi import APIRouter, Depends, HTTPException, Query, Request, status
from sqlalchemy import select, func, or_, desc
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.api.deps import get_current_user, bearer
from app.core.database import get_db
from app.core.security import decode_token
from app.models import User, Blueprint, BlueprintImage, Favorite, Comment, Tag, BlueprintTag
from app.schemas import (
    BlueprintCreate, BlueprintUpdate, BlueprintOut, BlueprintDetail,
    BlueprintListOut, CommentCreate, CommentOut, UserOut,
)

router = APIRouter(prefix="/api/blueprints", tags=["blueprints"])

_SPECIAL_PATTERN = re.compile(r"[^a-z0-9-]")


def _slugify(title: str) -> str:
    slug = title.lower().strip()
    slug = _SPECIAL_PATTERN.sub("-", slug)
    slug = re.sub(r"-{2,}", "-", slug)
    return slug.strip("-")


# ────────────────────────── Optional auth ──────────────────────────

async def _optional_user(
    request: Request,
    db: AsyncSession = Depends(get_db),
):
    """Try to get current user from token, return None if not authenticated."""
    auth_header = request.headers.get("Authorization")
    if not auth_header or not auth_header.startswith("Bearer "):
        return None
    token = auth_header.split(" ", 1)[1]
    claims = decode_token(token)
    if claims.get("type") != "access":
        return None
    user_id = claims.get("sub")
    if not user_id:
        return None
    result = await db.execute(select(User).where(User.id == user_id))
    return result.scalar_one_or_none()


# ────────────────────────── CREATE ──────────────────────────

@router.post("", response_model=BlueprintOut, status_code=201)
async def create_blueprint(
    payload: BlueprintCreate,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    blueprint = Blueprint(
        title=payload.title,
        slug=_slugify(payload.title),
        description=payload.description,
        difficulty=payload.difficulty,
        piece_count=payload.piece_count,
        category=payload.category,
        dimensions=payload.dimensions,
        part_list=payload.part_list,
        is_published=payload.is_published,
        author_id=current_user.id,
    )
    db.add(blueprint)
    await db.commit()
    await db.refresh(blueprint)

    # eager load author
    await db.refresh(blueprint, attribute_names=["author"])
    return _to_blueprint_out(blueprint)


# ────────────────────────── READ ──────────────────────────

@router.get("/{blueprint_id}", response_model=BlueprintDetail)
async def get_blueprint(
    blueprint_id: str,
    db: AsyncSession = Depends(get_db),
    current_user: Optional[User] = Depends(_optional_user),
):
    result = await db.execute(
        select(Blueprint)
        .options(
            selectinload(Blueprint.author),
            selectinload(Blueprint.images),
            selectinload(Blueprint.tags).selectinload(BlueprintTag.tag),
        )
        .where(Blueprint.id == blueprint_id)
    )
    blueprint = result.scalar_one_or_none()
    if not blueprint:
        raise HTTPException(status_code=404, detail="Blueprint not found")

    # Check if current user has favorited (before commit)
    is_fav = False
    if current_user:
        fav_result = await db.execute(
            select(Favorite).where(
                Favorite.user_id == current_user.id,
                Favorite.blueprint_id == blueprint_id,
            )
        )
        is_fav = fav_result.scalar_one_or_none() is not None

    # Count favorites
    fav_count_result = await db.execute(
        select(func.count()).where(Favorite.blueprint_id == blueprint_id)
    )
    fav_count = fav_count_result.scalar() or 0

    # Increment view count
    blueprint.view_count += 1

    # Build response (view_count is already incremented on the object)
    response = _to_blueprint_detail(blueprint, is_favorited=is_fav, favorite_count=fav_count)
    await db.commit()

    return response


# ────────────────────────── UPDATE ──────────────────────────

@router.put("/{blueprint_id}", response_model=BlueprintOut)
async def update_blueprint(
    blueprint_id: str,
    payload: BlueprintUpdate,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    result = await db.execute(
        select(Blueprint).options(selectinload(Blueprint.author))
        .where(Blueprint.id == blueprint_id)
    )
    blueprint = result.scalar_one_or_none()
    if not blueprint:
        raise HTTPException(status_code=404, detail="Blueprint not found")
    if blueprint.author_id != current_user.id:
        raise HTTPException(status_code=403, detail="Not authorized to edit this blueprint")

    update_data = payload.model_dump(exclude_unset=True)
    if "title" in update_data:
        update_data["slug"] = _slugify(update_data["title"])
    for field, value in update_data.items():
        setattr(blueprint, field, value)

    await db.commit()
    await db.refresh(blueprint)
    return _to_blueprint_out(blueprint)


# ────────────────────────── DELETE ──────────────────────────

@router.delete("/{blueprint_id}", status_code=204)
async def delete_blueprint(
    blueprint_id: str,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    result = await db.execute(select(Blueprint).where(Blueprint.id == blueprint_id))
    blueprint = result.scalar_one_or_none()
    if not blueprint:
        raise HTTPException(status_code=404, detail="Blueprint not found")
    if blueprint.author_id != current_user.id:
        raise HTTPException(status_code=403, detail="Not authorized to delete this blueprint")

    await db.delete(blueprint)
    await db.commit()


# ────────────────────────── LIST ──────────────────────────

@router.get("", response_model=BlueprintListOut)
async def list_blueprints(
    page: int = Query(default=1, ge=1),
    size: int = Query(default=12, ge=1, le=50),
    q: Optional[str] = Query(default=None, description="搜索关键词"),
    category: Optional[str] = Query(default=None),
    sort: Optional[str] = Query(default="new", description="new | popular"),
    db: AsyncSession = Depends(get_db),
):
    base_query = select(Blueprint).options(
        selectinload(Blueprint.author),
        selectinload(Blueprint.tags).selectinload(BlueprintTag.tag),
    )

    # Search
    if q:
        base_query = base_query.where(
            or_(
                Blueprint.title.ilike(f"%{q}%"),
                Blueprint.description.ilike(f"%{q}%"),
            )
        )

    # Category filter
    if category:
        base_query = base_query.where(Blueprint.category == category)

    # Count
    count_query = select(func.count()).select_from(base_query.subquery())
    total = (await db.execute(count_query)).scalar() or 0

    # Sort
    if sort == "popular":
        base_query = base_query.order_by(desc(Blueprint.view_count))
    else:
        base_query = base_query.order_by(desc(Blueprint.created_at))

    # Page
    offset = (page - 1) * size
    base_query = base_query.offset(offset).limit(size)

    result = await db.execute(base_query)
    blueprints = result.scalars().all()

    return BlueprintListOut(
        items=[_to_blueprint_out(bp) for bp in blueprints],
        total=total,
        page=page,
        page_size=size,
    )


# ────────────────────────── FAVORITE ──────────────────────────

@router.post("/{blueprint_id}/favorite", status_code=201)
async def favorite_blueprint(
    blueprint_id: str,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    # Check blueprint exists
    bp = await db.get(Blueprint, blueprint_id)
    if not bp:
        raise HTTPException(status_code=404, detail="Blueprint not found")

    # Check not already favorited
    existing = await db.execute(
        select(Favorite).where(
            Favorite.user_id == current_user.id,
            Favorite.blueprint_id == blueprint_id,
        )
    )
    if existing.scalar_one_or_none():
        raise HTTPException(status_code=409, detail="Already favorited")

    fav = Favorite(user_id=current_user.id, blueprint_id=blueprint_id)
    db.add(fav)
    await db.commit()
    return {"detail": "Favorited"}


@router.delete("/{blueprint_id}/favorite", status_code=204)
async def unfavorite_blueprint(
    blueprint_id: str,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    result = await db.execute(
        select(Favorite).where(
            Favorite.user_id == current_user.id,
            Favorite.blueprint_id == blueprint_id,
        )
    )
    fav = result.scalar_one_or_none()
    if not fav:
        raise HTTPException(status_code=404, detail="Not favorited")

    await db.delete(fav)
    await db.commit()


# ────────────────────────── COMMENTS ──────────────────────────

@router.post("/{blueprint_id}/comments", response_model=CommentOut, status_code=201)
async def create_comment(
    blueprint_id: str,
    payload: CommentCreate,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    bp = await db.get(Blueprint, blueprint_id)
    if not bp:
        raise HTTPException(status_code=404, detail="Blueprint not found")

    comment = Comment(
        blueprint_id=blueprint_id,
        user_id=current_user.id,
        content=payload.content,
    )
    db.add(comment)
    await db.commit()
    # Re-query with eager-loaded user
    result = await db.execute(
        select(Comment)
        .options(selectinload(Comment.user))
        .where(Comment.id == comment.id)
    )
    comment = result.scalar_one()
    return _to_comment_out(comment)


@router.get("/{blueprint_id}/comments", response_model=list[CommentOut])
async def list_comments(
    blueprint_id: str,
    db: AsyncSession = Depends(get_db),
):
    result = await db.execute(
        select(Comment)
        .options(selectinload(Comment.user))
        .where(Comment.blueprint_id == blueprint_id)
        .order_by(Comment.created_at)
    )
    comments = result.scalars().all()
    return [_to_comment_out(c) for c in comments]


# ────────────────────────── Helpers ──────────────────────────

def _to_blueprint_out(bp: Blueprint) -> dict:
    tags = _get_tag_names(bp)
    author = _try_get_author(bp)
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
        "is_published": bp.is_published,
        "created_at": bp.created_at.isoformat() if bp.created_at else "",
        "updated_at": bp.updated_at.isoformat() if bp.updated_at else "",
        "author": author,
        "images": [],
        "tags": tags,
    }


def _to_blueprint_detail(bp: Blueprint, is_favorited: bool = False, favorite_count: int = 0) -> dict:
    data = _to_blueprint_out(bp)
    data["is_favorited"] = is_favorited
    data["favorite_count"] = favorite_count
    return data


def _to_user_out(user: User) -> dict:
    return {
        "id": user.id,
        "username": user.username,
        "email": user.email,
        "avatar_url": user.avatar_url,
        "bio": user.bio,
        "created_at": user.created_at.isoformat() if user.created_at else "",
    }


def _to_comment_out(comment: Comment) -> dict:
    user = None
    try:
        if comment.user:
            user = _to_user_out(comment.user)
    except Exception:
        pass
    return {
        "id": comment.id,
        "blueprint_id": comment.blueprint_id,
        "user_id": comment.user_id,
        "content": comment.content,
        "created_at": comment.created_at.isoformat() if comment.created_at else "",
        "user": user,
    }


def _get_tag_names(bp: Blueprint) -> list[str]:
    """Safely get tag names. Returns empty list if tags not loaded."""
    try:
        tags = bp.tags
    except Exception:
        return []
    if not tags:
        return []
    result = []
    for bt in tags:
        try:
            result.append(bt.tag.name)
        except Exception:
            continue
    return result


def _try_get_author(bp: Blueprint) -> dict | None:
    """Safely get author dict. Returns None if author not loaded."""
    try:
        author = bp.author
    except Exception:
        return None
    if author is None:
        return None
    try:
        return _to_user_out(author)
    except Exception:
        return None
