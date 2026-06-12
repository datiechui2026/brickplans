from pydantic import BaseModel, EmailStr, Field


# ── Auth ──

class UserRegister(BaseModel):
    username: str = Field(min_length=2, max_length=30)
    email: EmailStr
    password: str = Field(min_length=6, max_length=128)


class UserLogin(BaseModel):
    email: EmailStr
    password: str


# ── User ──

class UserOut(BaseModel):
    id: str
    username: str
    email: str
    avatar_url: str | None = None
    bio: str | None = None
    is_admin: bool = False
    created_at: str

    model_config = {"from_attributes": True}


class TokenResponse(BaseModel):
    access_token: str
    refresh_token: str
    token_type: str = "bearer"
    user: UserOut | None = None


class RefreshRequest(BaseModel):
    refresh_token: str


# ── Blueprint ──

class BlueprintCreate(BaseModel):
    title: str = Field(min_length=1, max_length=100)
    description: str | None = None
    difficulty: int | None = Field(default=None, ge=1, le=5)
    piece_count: int | None = None
    category: str | None = None
    dimensions: str | None = None
    part_list: dict | None = None
    is_published: bool = True


class BlueprintUpdate(BaseModel):
    title: str | None = Field(default=None, min_length=1, max_length=100)
    description: str | None = None
    difficulty: int | None = Field(default=None, ge=1, le=5)
    piece_count: int | None = None
    category: str | None = None
    dimensions: str | None = None
    part_list: dict | None = None
    is_published: bool | None = None


class BlueprintOut(BaseModel):
    id: str
    author_id: str
    title: str
    slug: str = ""
    description: str | None = None
    difficulty: int | None = None
    piece_count: int | None = None
    category: str | None = None
    dimensions: str | None = None
    part_list: dict | None = None
    view_count: int = 0
    like_count: int = 0
    favorite_count: int = 0
    is_liked: bool = False
    cover_url: str | None = None
    is_published: bool = False
    created_at: str
    updated_at: str
    author: "UserOut | None" = None
    images: list["BlueprintImageOut"] = []
    tags: list[str] = []

    model_config = {"from_attributes": True}


class BlueprintDetail(BaseModel):
    """图纸详情（含收藏状态）"""
    id: str
    author_id: str
    title: str
    slug: str = ""
    description: str | None = None
    difficulty: int | None = None
    piece_count: int | None = None
    category: str | None = None
    dimensions: str | None = None
    part_list: dict | None = None
    view_count: int = 0
    favorite_count: int = 0
    is_favorited: bool = False
    is_published: bool = False
    created_at: str
    updated_at: str
    author: "UserOut | None" = None
    images: list["BlueprintImageOut"] = []
    tags: list[str] = []

    model_config = {"from_attributes": True}


class BlueprintImageOut(BaseModel):
    id: str
    url: str
    sort_order: int = 0
    is_cover: bool = False

    model_config = {"from_attributes": True}


class BlueprintListOut(BaseModel):
    items: list[BlueprintOut]
    total: int
    page: int
    page_size: int


# ── Comment ──

class CommentCreate(BaseModel):
    content: str = Field(min_length=1, max_length=2000)
    parent_id: str | None = None


class CommentOut(BaseModel):
    id: str
    blueprint_id: str
    user_id: str
    parent_id: str | None = None
    content: str
    created_at: str
    user: UserOut | None = None

    model_config = {"from_attributes": True}


# ── Notification ──

class NotificationOut(BaseModel):
    id: str
    user_id: str
    actor_id: str | None = None
    type: str
    blueprint_id: str | None = None
    comment_id: str | None = None
    payload: dict | None = None
    is_read: bool = False
    created_at: str
    read_at: str | None = None
    actor: UserOut | None = None

    model_config = {"from_attributes": True}


class NotificationListOut(BaseModel):
    items: list[NotificationOut]
    total: int
    unread_count: int
    page: int
    page_size: int


# ── Report ──

class ReportCreate(BaseModel):
    blueprint_id: str
    reason: str = Field(min_length=1, max_length=20)
    detail: str | None = None


class ReportOut(BaseModel):
    id: str
    reporter_id: str
    blueprint_id: str
    reason: str
    detail: str | None = None
    status: str
    created_at: str

    model_config = {"from_attributes": True}


# ── Stats ──

class StatsResponse(BaseModel):
    total_blueprints: int = 0
    total_users: int = 0
    total_favorites: int = 0
    total_pieces: int = 0
    total_views: int = 0
    total_likes: int = 0
