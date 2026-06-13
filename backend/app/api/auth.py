import os
import random
import uuid as uuid_lib

from fastapi import APIRouter, Depends, HTTPException, status, UploadFile, File
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from pydantic import BaseModel, Field

from app.core.database import get_db
from app.core.security import hash_password, verify_password, create_access_token, create_refresh_token
from app.models import User
from app.schemas import UserRegister, UserLogin, TokenResponse, UserOut, RefreshRequest
from app.services.storage import get_storage
from app.api.deps import get_current_user
from app.api.blueprints import _to_user_out

router = APIRouter(prefix="/api/auth", tags=["auth"])

# Request schemas for user settings
class UserUpdateRequest(BaseModel):
    username: str | None = Field(default=None, min_length=2, max_length=30)
    bio: str | None = None
    avatar_url: str | None = None

class PasswordChangeRequest(BaseModel):
    current_password: str
    new_password: str = Field(min_length=6, max_length=128)

# Allowed avatar image types and max size (2MB)
ALLOWED_AVATAR_TYPES = {"image/jpeg", "image/png", "image/webp", "image/gif"}
MAX_AVATAR_SIZE = 2 * 1024 * 1024  # 2MB


@router.post("/register", response_model=TokenResponse, status_code=201)
async def register(payload: UserRegister, db: AsyncSession = Depends(get_db)):
    # Check duplicate email
    result = await db.execute(select(User).where(User.email == payload.email))
    if result.scalar_one_or_none():
        raise HTTPException(status_code=409, detail="Email already registered")

    # Check duplicate username
    result = await db.execute(select(User).where(User.username == payload.username))
    if result.scalar_one_or_none():
        raise HTTPException(status_code=409, detail="Username already taken")

    user = User(
        username=payload.username,
        email=payload.email,
        password_hash=hash_password(payload.password),
    )
    # Pick random preset avatar (01-20)
    preset_num = random.randint(1, 20)
    user.avatar_url = f"/avatars/presets/{preset_num:02d}.png"
    db.add(user)
    await db.flush()

    access_token = create_access_token(user.id)
    refresh_token = create_refresh_token(user.id)

    return TokenResponse(access_token=access_token, refresh_token=refresh_token, user=_to_user_out(user))


@router.post("/login", response_model=TokenResponse)
async def login(payload: UserLogin, db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(User).where(User.email == payload.email))
    user = result.scalar_one_or_none()

    if not user or not verify_password(payload.password, user.password_hash):
        raise HTTPException(status_code=401, detail="Invalid email or password")

    access_token = create_access_token(user.id)
    refresh_token = create_refresh_token(user.id)

    return TokenResponse(access_token=access_token, refresh_token=refresh_token, user=_to_user_out(user))


@router.post("/refresh", response_model=TokenResponse)
async def refresh(payload: RefreshRequest):
    try:
        claims = decode_token(payload.refresh_token)
        if claims.get("type") != "refresh":
            raise HTTPException(status_code=401, detail="Invalid token type")
    except Exception:
        raise HTTPException(status_code=401, detail="Invalid or expired token")

    access_token = create_access_token(claims["sub"])
    refresh_token = create_refresh_token(claims["sub"])

    return TokenResponse(access_token=access_token, refresh_token=refresh_token)


@router.get("/me", response_model=UserOut)
async def get_me(current_user: User = Depends(get_current_user)):
    return _to_user_out(current_user)


@router.put("/me", response_model=UserOut)
async def update_me(
    payload: UserUpdateRequest,
    current_user: User = Depends(get_current_user),
    db: AsyncSession = Depends(get_db),
):
    """Update current user's username and/or bio."""
    if payload.username is not None and payload.username != current_user.username:
        # Check uniqueness: another user shouldn't have this username
        result = await db.execute(
            select(User).where(User.username == payload.username, User.id != current_user.id)
        )
        if result.scalar_one_or_none():
            raise HTTPException(status_code=409, detail="Username already taken")
        current_user.username = payload.username

    if payload.bio is not None:
        current_user.bio = payload.bio

    if payload.avatar_url is not None:
        current_user.avatar_url = payload.avatar_url

    await db.commit()
    await db.refresh(current_user)
    return _to_user_out(current_user)


@router.put("/password")
async def change_password(
    payload: PasswordChangeRequest,
    current_user: User = Depends(get_current_user),
    db: AsyncSession = Depends(get_db),
):
    """Change password. Verify current password, then set new one."""
    if not verify_password(payload.current_password, current_user.password_hash):
        raise HTTPException(status_code=400, detail="Current password is incorrect")

    current_user.password_hash = hash_password(payload.new_password)
    await db.commit()
    return {"message": "Password updated"}


@router.post("/avatar")
async def upload_avatar(
    file: UploadFile = File(...),
    current_user: User = Depends(get_current_user),
    db: AsyncSession = Depends(get_db),
):
    """Upload avatar image. Max 2MB, jpg/png/webp/gif only."""
    # Validate content type
    if file.content_type not in ALLOWED_AVATAR_TYPES:
        raise HTTPException(
            status_code=400,
            detail=f"Unsupported file type: {file.content_type}. Allowed: jpg, png, webp, gif",
        )

    # Read and validate size
    contents = await file.read()
    if len(contents) > MAX_AVATAR_SIZE:
        raise HTTPException(status_code=400, detail="File too large. Maximum size is 2MB.")

    filename = file.filename or f"{uuid_lib.uuid4()}.png"
    stored = await get_storage().upload(contents, filename, file.content_type or "image/png", prefix="avatars")

    # Update user's avatar_url
    current_user.avatar_url = stored.url
    await db.commit()
    await db.refresh(current_user)

    return {"avatar_url": avatar_url}


@router.get("/avatars")
def get_preset_avatars():
    """Return list of preset avatar URLs (public)."""
    return {
        "avatars": [
            {"id": f"preset-{i:02d}", "url": f"/avatars/presets/{i:02d}.png"}
            for i in range(1, 21)
        ]
    }
