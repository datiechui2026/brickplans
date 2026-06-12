import uuid
from datetime import datetime, timezone

from sqlalchemy import String, Text, Integer, Boolean, Float, ForeignKey, DateTime, UniqueConstraint, Index, JSON, Uuid
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.core.database import Base


def new_uuid() -> str:
    return str(uuid.uuid4())


def now() -> datetime:
    return datetime.now(timezone.utc)


class User(Base):
    __tablename__ = "users"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=new_uuid)
    username: Mapped[str] = mapped_column(String(30), unique=True, nullable=False, index=True)
    email: Mapped[str] = mapped_column(String(255), unique=True, nullable=False)
    password_hash: Mapped[str] = mapped_column(String(255), nullable=False)
    avatar_url: Mapped[str | None] = mapped_column(String(500))
    bio: Mapped[str | None] = mapped_column(Text)
    is_admin: Mapped[bool] = mapped_column(Boolean, default=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=now)

    blueprints: Mapped[list["Blueprint"]] = relationship(back_populates="author", cascade="all, delete-orphan")
    comments: Mapped[list["Comment"]] = relationship(back_populates="user", cascade="all, delete-orphan")
    reports: Mapped[list["Report"]] = relationship(back_populates="reporter", cascade="all, delete-orphan")
    likes: Mapped[list["Like"]] = relationship(back_populates="user", cascade="all, delete-orphan")


class Blueprint(Base):
    __tablename__ = "blueprints"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=new_uuid)
    author_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    title: Mapped[str] = mapped_column(String(100), nullable=False)
    slug: Mapped[str] = mapped_column(String(120), nullable=False, index=True)
    description: Mapped[str | None] = mapped_column(Text)
    difficulty: Mapped[int | None] = mapped_column(Integer)
    piece_count: Mapped[int | None] = mapped_column(Integer)
    category: Mapped[str | None] = mapped_column(String(30), index=True)
    dimensions: Mapped[str | None] = mapped_column(String(50))
    part_list: Mapped[dict | None] = mapped_column(JSON)
    view_count: Mapped[int] = mapped_column(Integer, default=0)
    like_count: Mapped[int] = mapped_column(Integer, default=0)
    is_published: Mapped[bool] = mapped_column(Boolean, default=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=now)
    updated_at: Mapped[datetime] = mapped_column(DateTime, default=now, onupdate=now)

    author: Mapped["User"] = relationship(back_populates="blueprints")
    images: Mapped[list["BlueprintImage"]] = relationship(back_populates="blueprint", cascade="all, delete-orphan", order_by="BlueprintImage.sort_order")
    tags: Mapped[list["BlueprintTag"]] = relationship(back_populates="blueprint", cascade="all, delete-orphan")
    comments: Mapped[list["Comment"]] = relationship(back_populates="blueprint", cascade="all, delete-orphan")
    favorites: Mapped[list["Favorite"]] = relationship(back_populates="blueprint", cascade="all, delete-orphan")
    likes: Mapped[list["Like"]] = relationship(back_populates="blueprint", cascade="all, delete-orphan")
    reports: Mapped[list["Report"]] = relationship(back_populates="blueprint", cascade="all, delete-orphan")

    __table_args__ = (
        Index("idx_blueprints_created", "created_at"),
        Index("idx_blueprints_category", "category"),
    )


class BlueprintImage(Base):
    __tablename__ = "blueprint_images"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=new_uuid)
    blueprint_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), ForeignKey("blueprints.id", ondelete="CASCADE"), nullable=False)
    url: Mapped[str] = mapped_column(String(500), nullable=False)
    sort_order: Mapped[int] = mapped_column(Integer, default=0)
    is_cover: Mapped[bool] = mapped_column(Boolean, default=False)

    blueprint: Mapped["Blueprint"] = relationship(back_populates="images")


class Tag(Base):
    __tablename__ = "tags"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=new_uuid)
    name: Mapped[str] = mapped_column(String(30), unique=True, nullable=False)

    blueprint_tags: Mapped[list["BlueprintTag"]] = relationship(back_populates="tag")


class BlueprintTag(Base):
    __tablename__ = "blueprint_tags"

    blueprint_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), ForeignKey("blueprints.id", ondelete="CASCADE"), primary_key=True)
    tag_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), ForeignKey("tags.id", ondelete="CASCADE"), primary_key=True)

    blueprint: Mapped["Blueprint"] = relationship(back_populates="tags")
    tag: Mapped["Tag"] = relationship(back_populates="blueprint_tags")


class Favorite(Base):
    __tablename__ = "favorites"

    user_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), ForeignKey("users.id", ondelete="CASCADE"), primary_key=True)
    blueprint_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), ForeignKey("blueprints.id", ondelete="CASCADE"), primary_key=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=now)

    blueprint: Mapped["Blueprint"] = relationship(back_populates="favorites")


class Like(Base):
    __tablename__ = "likes"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=new_uuid)
    user_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    blueprint_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), ForeignKey("blueprints.id", ondelete="CASCADE"), nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=now)

    user: Mapped["User"] = relationship(back_populates="likes")
    blueprint: Mapped["Blueprint"] = relationship(back_populates="likes")

    __table_args__ = (
        UniqueConstraint("user_id", "blueprint_id", name="uq_like_user_blueprint"),
    )


class Comment(Base):
    __tablename__ = "comments"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=new_uuid)
    blueprint_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), ForeignKey("blueprints.id", ondelete="CASCADE"), nullable=False)
    user_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    content: Mapped[str] = mapped_column(Text, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=now)

    blueprint: Mapped["Blueprint"] = relationship(back_populates="comments")
    user: Mapped["User"] = relationship(back_populates="comments")


class Report(Base):
    __tablename__ = "reports"

    id: Mapped[str] = mapped_column(Uuid(as_uuid=False), primary_key=True, default=new_uuid)
    reporter_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    blueprint_id: Mapped[str] = mapped_column(Uuid(as_uuid=False), ForeignKey("blueprints.id", ondelete="CASCADE"), nullable=False)
    reason: Mapped[str] = mapped_column(String(20), nullable=False)
    detail: Mapped[str | None] = mapped_column(Text)
    status: Mapped[str] = mapped_column(String(20), default="pending")
    created_at: Mapped[datetime] = mapped_column(DateTime, default=now)

    reporter: Mapped["User"] = relationship(back_populates="reports")
    blueprint: Mapped["Blueprint"] = relationship(back_populates="reports")

    __table_args__ = (
        UniqueConstraint("reporter_id", "blueprint_id", name="uq_report_reporter_blueprint"),
    )
