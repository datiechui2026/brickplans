from pydantic import BaseModel, EmailStr, Field


# ── Auth ──

class UserRegister(BaseModel):
    username: str = Field(min_length=2, max_length=30)
    email: EmailStr
    password: str = Field(min_length=6, max_length=128)


class UserLogin(BaseModel):
    email: EmailStr
    password: str


class TokenResponse(BaseModel):
    access_token: str
    refresh_token: str
    token_type: str = "bearer"


class RefreshRequest(BaseModel):
    refresh_token: str


# ── User ──

class UserOut(BaseModel):
    id: str
    username: str
    email: str
    avatar_url: str | None = None
    bio: str | None = None
    created_at: str

    model_config = {"from_attributes": True}


# ── Blueprint ──

class BlueprintCreate(BaseModel):
    title: str = Field(min_length=1, max_length=100)
    description: str | None = None
    difficulty: int | None = Field(default=None, ge=1, le=5)
    piece_count: int | None = None
    category: str | None = None
    dimensions: str | None = None
    part_list: dict | None = None
    is_published: bool = False


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
    description: str | None = None
    difficulty: int | None = None
    piece_count: int | None = None
    category: str | None = None
    dimensions: str | None = None
    part_list: dict | None = None
    view_count: int = 0
    is_published: bool = False
    created_at: str
    updated_at: str
    author: UserOut | None = None
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


class CommentOut(BaseModel):
    id: str
    blueprint_id: str
    user_id: str
    content: str
    created_at: str
    user: UserOut | None = None

    model_config = {"from_attributes": True}
