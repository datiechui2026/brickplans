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
from jose.exceptions import ExpiredSignatureError, JWTError
from app.models import User, Blueprint, BlueprintImage, Favorite, Comment, Tag, BlueprintTag, Like, Notification
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
    try:
        claims = decode_token(token)
    except (ExpiredSignatureError, JWTError):
        return None
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

    # If unpublished and requesting user is NOT the author, return 404
    if not blueprint.is_published:
        if not current_user or current_user.id != blueprint.author_id:
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
    tag: Optional[str] = Query(default=None, description="按标签名筛选"),
    db: AsyncSession = Depends(get_db),
    current_user: Optional[User] = Depends(_optional_user),
):
    base_query = select(Blueprint).options(
        selectinload(Blueprint.author),
        selectinload(Blueprint.tags).selectinload(BlueprintTag.tag),
        selectinload(Blueprint.images),
    )

    # Only show published blueprints on public listings
    base_query = base_query.where(Blueprint.is_published == True)

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

    # Tag filter
    if tag:
        base_query = base_query.where(
            Blueprint.tags.any(BlueprintTag.tag.has(Tag.name == tag))
        )

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

    # Compute per-blueprint counts in bulk
    bp_ids = [bp.id for bp in blueprints]
    items = []
    if bp_ids:
        # Bulk favorite/like counts
        fav_counts = {}
        like_counts = {}
        user_liked = set()
        user_favorited = set()

        fav_rows = (await db.execute(
            select(Favorite.blueprint_id, func.count().label("cnt"))
            .where(Favorite.blueprint_id.in_(bp_ids))
            .group_by(Favorite.blueprint_id)
        )).all()
        for row in fav_rows:
            fav_counts[row.blueprint_id] = row.cnt

        like_rows = (await db.execute(
            select(Like.blueprint_id, func.count().label("cnt"))
            .where(Like.blueprint_id.in_(bp_ids))
            .group_by(Like.blueprint_id)
        )).all()
        for row in like_rows:
            like_counts[row.blueprint_id] = row.cnt

        if current_user:
            liked_rows = (await db.execute(
                select(Like.blueprint_id).where(
                    Like.blueprint_id.in_(bp_ids),
                    Like.user_id == current_user.id,
                )
            )).scalars().all()
            user_liked = set(liked_rows)

            fav_user_rows = (await db.execute(
                select(Favorite.blueprint_id).where(
                    Favorite.blueprint_id.in_(bp_ids),
                    Favorite.user_id == current_user.id,
                )
            )).scalars().all()
            user_favorited = set(fav_user_rows)

        for bp in blueprints:
            items.append(_to_blueprint_out(bp,
                favorite_count=fav_counts.get(bp.id, 0),
                like_count=like_counts.get(bp.id, 0),
                is_liked=bp.id in user_liked,
                is_favorited=bp.id in user_favorited,
            ))
    else:
        items = [_to_blueprint_out(bp) for bp in blueprints]

    return BlueprintListOut(
        items=items,
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
    _add_notification(
        db,
        user_id=bp.author_id,
        actor_id=current_user.id,
        type="favorite",
        blueprint_id=blueprint_id,
        payload={"blueprint_title": bp.title},
    )
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


# ────────────────────────── LIKE ──────────────────────────

@router.post("/{blueprint_id}/like", status_code=201)
async def like_blueprint(
    blueprint_id: str,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    bp = await db.get(Blueprint, blueprint_id)
    if not bp:
        raise HTTPException(status_code=404, detail="Blueprint not found")

    existing = await db.execute(
        select(Like).where(
            Like.user_id == current_user.id,
            Like.blueprint_id == blueprint_id,
        )
    )
    if existing.scalar_one_or_none():
        raise HTTPException(status_code=409, detail="Already liked")

    like = Like(user_id=current_user.id, blueprint_id=blueprint_id)
    db.add(like)
    bp.like_count += 1
    _add_notification(
        db,
        user_id=bp.author_id,
        actor_id=current_user.id,
        type="like",
        blueprint_id=blueprint_id,
        payload={"blueprint_title": bp.title},
    )
    await db.commit()
    return {"detail": "Liked", "like_count": bp.like_count}


@router.delete("/{blueprint_id}/like", status_code=204)
async def unlike_blueprint(
    blueprint_id: str,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    result = await db.execute(
        select(Like).where(
            Like.user_id == current_user.id,
            Like.blueprint_id == blueprint_id,
        )
    )
    like = result.scalar_one_or_none()
    if not like:
        raise HTTPException(status_code=404, detail="Not liked")

    bp = await db.get(Blueprint, blueprint_id)
    if bp and bp.like_count > 0:
        bp.like_count -= 1

    await db.delete(like)
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

    parent = None
    if payload.parent_id:
        parent = await db.get(Comment, payload.parent_id)
        if not parent or parent.blueprint_id != blueprint_id:
            raise HTTPException(status_code=404, detail="Parent comment not found")
        if parent.parent_id:
            raise HTTPException(status_code=400, detail="Only one-level replies are supported")

    comment = Comment(
        blueprint_id=blueprint_id,
        user_id=current_user.id,
        parent_id=payload.parent_id,
        content=payload.content,
    )
    db.add(comment)
    await db.flush()

    if parent:
        _add_notification(
            db,
            user_id=parent.user_id,
            actor_id=current_user.id,
            type="comment_reply",
            blueprint_id=blueprint_id,
            comment_id=comment.id,
            payload={"blueprint_title": bp.title, "comment_excerpt": payload.content[:80]},
        )
    else:
        _add_notification(
            db,
            user_id=bp.author_id,
            actor_id=current_user.id,
            type="comment",
            blueprint_id=blueprint_id,
            comment_id=comment.id,
            payload={"blueprint_title": bp.title, "comment_excerpt": payload.content[:80]},
        )

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

def _add_notification(
    db: AsyncSession,
    *,
    user_id: str,
    actor_id: str,
    type: str,
    blueprint_id: str | None = None,
    comment_id: str | None = None,
    payload: dict | None = None,
) -> None:
    if user_id == actor_id:
        return
    db.add(Notification(
        user_id=user_id,
        actor_id=actor_id,
        type=type,
        blueprint_id=blueprint_id,
        comment_id=comment_id,
        payload=payload,
    ))


def _to_blueprint_out(bp: Blueprint,
                       favorite_count: int = 0,
                       like_count: int = 0,
                       is_liked: bool = False,
                       is_favorited: bool = False) -> dict:
    tags = _get_tag_names(bp)
    author = _try_get_author(bp)
    moderation_status = "审核中" if not bp.is_published else None

    # Compute cover_url — prefer marked cover, but skip PDFs
    images = _try_get_images(bp)
    cover_url = None
    for img in images:
        if img.get("is_cover") and img.get("file_type", "image") != "pdf":
            cover_url = img["url"]
            break
    if not cover_url:
        # Fallback: first non-PDF image
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
        "like_count": bp.like_count if not like_count else like_count,
        "favorite_count": favorite_count,
        "is_liked": is_liked,
        "cover_url": cover_url,
        "is_published": bp.is_published,
        "created_at": bp.created_at.isoformat() if bp.created_at else "",
        "updated_at": bp.updated_at.isoformat() if bp.updated_at else "",
        "author": author,
        "images": images,
        "tags": tags,
        "moderation_status": moderation_status,
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
        "is_admin": user.is_admin,
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
        "parent_id": comment.parent_id,
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


def _try_get_images(bp: Blueprint) -> list[dict]:
    """Safely get images list. Returns empty list if images not loaded."""
    try:
        images = bp.images
    except Exception:
        return []
    if not images:
        return []
    result = []
    for img in images:
        try:
            result.append({
                "id": img.id,
                "url": img.url,
                "object_key": img.object_key,
                "sort_order": img.sort_order,
                "is_cover": img.is_cover,
                "file_type": getattr(img, 'file_type', 'image') or 'image',
            })
        except Exception:
            continue
    return result
