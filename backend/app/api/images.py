"""
图片上传 & 管理 API

POST   /api/blueprints/{id}/images         — 多图上传（作者操作）
DELETE /api/blueprints/{id}/images/{img_id} — 删除图片（作者操作）
"""
from typing import List

from fastapi import APIRouter, Body, Depends, File, HTTPException, UploadFile, status
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_user
from app.core.database import get_db
from app.models import Blueprint, BlueprintImage, User
from app.services.storage import get_storage

router = APIRouter(prefix="/api/blueprints", tags=["images"])

ALLOWED_EXTENSIONS = {"jpg", "jpeg", "png", "webp"}
ALLOWED_MIME_TYPES = {"image/jpeg", "image/png", "image/webp"}
MAX_FILE_SIZE = 10 * 1024 * 1024  # 10 MB

MAGIC_BYTES = {
    b"\x89PNG\r\n\x1a\n": "png",
    b"\xff\xd8\xff": "jpeg",
    b"RIFF": "webp",
}


def _validate_file(file: UploadFile) -> bytes:
    """验证文件格式和大小，返回文件内容。"""
    # Extension
    ext = (file.filename or "").rsplit(".", 1)[-1].lower() if file.filename and "." in file.filename else ""
    if ext not in ALLOWED_EXTENSIONS:
        raise HTTPException(422, f"不支持格式 .{ext}，仅允许 {', '.join(sorted(ALLOWED_EXTENSIONS))}")

    # MIME
    if (file.content_type or "") not in ALLOWED_MIME_TYPES:
        raise HTTPException(422, f"不支持 MIME: {file.content_type}")

    # Read & size check
    content = file.file.read(MAX_FILE_SIZE + 1)
    if len(content) > MAX_FILE_SIZE:
        raise HTTPException(413, f"文件过大: {len(content) / 1024 / 1024:.1f}MB，最大 10MB")
    if not content:
        raise HTTPException(422, "文件为空")

    # Magic bytes
    for magic, ftype in MAGIC_BYTES.items():
        if content.startswith(magic):
            if ftype == "webp" and len(content) >= 12 and content[8:12] != b"WEBP":
                continue
            return content

    raise HTTPException(422, "文件内容非有效图片格式")


@router.post("/{blueprint_id}/images", status_code=201)
async def upload_images(
    blueprint_id: str,
    files: List[UploadFile] = File(..., description="图片文件"),
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    """上传图纸图片（仅作者）。支持多图，格式 jpg/png/webp，≤10MB。"""
    bp = await db.get(Blueprint, blueprint_id)
    if not bp:
        raise HTTPException(404, "Blueprint not found")
    if bp.author_id != current_user.id:
        raise HTTPException(403, "Only the blueprint author can upload images")

    # Validate all first
    contents = [_validate_file(f) for f in files]

    storage = get_storage()
    for sort_order, (file, content) in enumerate(zip(files, contents)):
        stored = await storage.upload(content, file.filename or "image", file.content_type or "image/png", prefix="blueprints")
        db.add(BlueprintImage(
            blueprint_id=blueprint_id,
            url=stored.url,
            object_key=stored.object_key,
            sort_order=sort_order,
        ))

    await db.flush()

    # Return all images for this blueprint
    result = await db.execute(
        select(BlueprintImage)
        .where(BlueprintImage.blueprint_id == blueprint_id)
        .order_by(BlueprintImage.sort_order)
    )
    images = result.scalars().all()
    await db.commit()

    return [
        {
            "id": img.id,
            "url": img.url,
            "object_key": img.object_key,
            "sort_order": img.sort_order,
            "is_cover": img.is_cover,
        }
        for img in images
    ]


@router.put("/{blueprint_id}/images/{image_id}/cover")
async def set_cover(
    blueprint_id: str,
    image_id: str,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    """设置封面图（仅作者）。"""
    bp = await db.get(Blueprint, blueprint_id)
    if not bp:
        raise HTTPException(404, "Blueprint not found")
    if bp.author_id != current_user.id:
        raise HTTPException(403, "Only the blueprint author can manage images")

    # Clear all covers for this blueprint
    result = await db.execute(
        select(BlueprintImage).where(BlueprintImage.blueprint_id == blueprint_id)
    )
    all_images = result.scalars().all()
    for img in all_images:
        img.is_cover = False

    # Set the target as cover
    target = await db.get(BlueprintImage, image_id)
    if not target or target.blueprint_id != blueprint_id:
        raise HTTPException(404, "Image not found")
    target.is_cover = True

    await db.commit()
    return {"message": "ok"}


@router.put("/{blueprint_id}/images/reorder")
async def reorder_images(
    blueprint_id: str,
    payload: dict = Body(...),
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    """重排图片顺序（仅作者）。"""
    bp = await db.get(Blueprint, blueprint_id)
    if not bp:
        raise HTTPException(404, "Blueprint not found")
    if bp.author_id != current_user.id:
        raise HTTPException(403, "Only the blueprint author can manage images")

    for item in payload.get("images", []):
        img_id = item.get("id")
        sort_order = item.get("sort_order", 0)
        result = await db.execute(
            select(BlueprintImage).where(
                BlueprintImage.id == img_id,
                BlueprintImage.blueprint_id == blueprint_id,
            )
        )
        img = result.scalar_one_or_none()
        if img:
            img.sort_order = sort_order
    await db.commit()
    return {"message": "ok"}


@router.delete("/{blueprint_id}/images/{image_id}", status_code=204)
async def delete_image(
    blueprint_id: str,
    image_id: str,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    """删除图纸图片（仅作者）。"""
    bp = await db.get(Blueprint, blueprint_id)
    if not bp:
        raise HTTPException(404, "Blueprint not found")
    if bp.author_id != current_user.id:
        raise HTTPException(403, "Only the blueprint author can manage images")

    image = await db.get(BlueprintImage, image_id)
    if not image or image.blueprint_id != blueprint_id:
        raise HTTPException(404, "Image not found")

    # Delete from storage
    storage = get_storage()
    try:
        await storage.delete(image.object_key or image.url)
    except Exception:
        pass  # best-effort: storage delete can fail without blocking DB cleanup

    await db.delete(image)
    await db.commit()
