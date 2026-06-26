"""
图片 & PDF 上传 API

POST   /api/blueprints/{id}/images  — 多文件上传（图片+PDF），自动压缩
DELETE /api/blueprints/{id}/images/{img_id} — 删除文件
"""
from io import BytesIO
from typing import List

from fastapi import APIRouter, Body, Depends, File, HTTPException, UploadFile, status
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.api.deps import get_current_user
from app.core.database import get_db
from app.models import Blueprint, BlueprintImage, User
from app.services.storage import get_storage

router = APIRouter(prefix="/api/blueprints", tags=["images"])

ALLOWED_IMAGE_EXTENSIONS = {"jpg", "jpeg", "png", "webp"}
ALLOWED_IMAGE_MIME_TYPES = {"image/jpeg", "image/png", "image/webp"}
ALLOWED_PDF_MIME_TYPES = {"application/pdf"}
MAX_FILE_SIZE = 20 * 1024 * 1024  # 20 MB

IMAGE_MAGIC = {
    b"\x89PNG\r\n\x1a\n": "png",
    b"\xff\xd8\xff": "jpeg",
    b"RIFF": "webp",
}
PDF_MAGIC = b"%PDF"

# ── Compression helpers ──

def _compress_image(content: bytes, max_dimension: int = 2048, quality: int = 80) -> bytes:
    """Compress image: resize if too large, re-encode to JPEG with quality."""
    try:
        from PIL import Image
        img = Image.open(BytesIO(content))
        # Convert RGBA to RGB for JPEG
        if img.mode in ("RGBA", "P"):
            img = img.convert("RGB")
        # Resize if larger than max_dimension
        w, h = img.size
        if max(w, h) > max_dimension:
            ratio = max_dimension / max(w, h)
            img = img.resize((int(w * ratio), int(h * ratio)), Image.LANCZOS)
        buf = BytesIO()
        img.save(buf, format="JPEG", quality=quality, optimize=True)
        return buf.getvalue()
    except Exception:
        # If Pillow fails, return original
        return content


def _compress_pdf(content: bytes) -> bytes:
    """Basic PDF compression via pypdf. Returns compressed bytes."""
    try:
        from pypdf import PdfReader, PdfWriter
        reader = PdfReader(BytesIO(content))
        writer = PdfWriter()
        for page in reader.pages:
            page.compress_content_streams()
            writer.add_page(page)
        buf = BytesIO()
        writer.write(buf)
        return buf.getvalue()
    except Exception:
        return content


def _validate_and_compress(file: UploadFile) -> tuple[bytes, str, str]:
    """
    Validate file, compress it, return (compressed_content, file_type, content_type).
    file_type: "image" or "pdf"
    """
    # Read content
    content = file.file.read(MAX_FILE_SIZE + 1)
    if len(content) > MAX_FILE_SIZE:
        raise HTTPException(413, f"文件过大: {len(content) / 1024 / 1024:.1f}MB，最大 20MB")
    if not content:
        raise HTTPException(422, "文件为空")

    # Detect PDF
    if content[:4] == PDF_MAGIC:
        compressed = _compress_pdf(content)
        return compressed, "pdf", "application/pdf"

    # Detect image by magic bytes
    for magic, ftype in IMAGE_MAGIC.items():
        if content.startswith(magic):
            if ftype == "webp" and len(content) >= 12 and content[8:12] != b"WEBP":
                continue
            compressed = _compress_image(content)
            return compressed, "image", "image/jpeg"

    # Fallback: check extension
    ext = (file.filename or "").rsplit(".", 1)[-1].lower() if file.filename and "." in file.filename else ""
    if ext in ALLOWED_IMAGE_EXTENSIONS:
        compressed = _compress_image(content)
        return compressed, "image", "image/jpeg"

    raise HTTPException(422, f"不支持的文件格式，仅允许 JPG/PNG/WebP/PDF")


@router.post("/{blueprint_id}/images", status_code=201)
async def upload_images(
    blueprint_id: str,
    files: List[UploadFile] = File(..., description="图片或PDF文件"),
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    """上传图纸文件（仅作者）。支持图片(jpg/png/webp)和PDF，自动压缩，≤20MB。"""
    bp = await db.get(Blueprint, blueprint_id)
    if not bp:
        raise HTTPException(404, "Blueprint not found")
    if bp.author_id != current_user.id:
        raise HTTPException(403, "Only the blueprint author can upload images")

    # Validate & compress all files first
    processed = [_validate_and_compress(f) for f in files]

    storage = get_storage()
    max_sort_order = await db.scalar(
        select(func.max(BlueprintImage.sort_order))
        .where(BlueprintImage.blueprint_id == blueprint_id)
    )
    next_sort_order = (-1 if max_sort_order is None else max_sort_order) + 1

    for offset, (file, (content, file_type, content_type)) in enumerate(zip(files, processed)):
        # Use appropriate extension
        ext = ".pdf" if file_type == "pdf" else ".jpg"
        filename = (file.filename or "file").rsplit(".", 1)[0] + ext
        stored = await storage.upload(content, filename, content_type, prefix="blueprints")
        db.add(BlueprintImage(
            blueprint_id=blueprint_id,
            url=stored.url,
            object_key=stored.object_key,
            sort_order=next_sort_order + offset,
            file_type=file_type,
        ))

    await db.flush()

    # Return all files for this blueprint
    result = await db.execute(
        select(BlueprintImage)
        .where(BlueprintImage.blueprint_id == blueprint_id)
        .order_by(BlueprintImage.sort_order, BlueprintImage.id)
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
            "file_type": getattr(img, 'file_type', 'image') or 'image',
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
    """重排文件顺序（仅作者）。"""
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
    """删除图纸文件（仅作者）。"""
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
        pass

    await db.delete(image)
    await db.commit()
