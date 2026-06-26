from sqlalchemy import inspect, text
from sqlalchemy.ext.asyncio import AsyncEngine

from app.core.database import Base


async def prepare_database(engine: AsyncEngine) -> None:
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
        await conn.run_sync(_ensure_comment_parent_id)
        await conn.run_sync(_ensure_blueprint_image_object_key)
        await conn.run_sync(_ensure_blueprint_image_file_type)


def _ensure_comment_parent_id(sync_conn) -> None:
    inspector = inspect(sync_conn)
    if "comments" not in inspector.get_table_names():
        return

    columns = {column["name"] for column in inspector.get_columns("comments")}
    if "parent_id" in columns:
        return

    sync_conn.execute(text("ALTER TABLE comments ADD COLUMN parent_id CHAR(32)"))
    sync_conn.execute(text("CREATE INDEX IF NOT EXISTS idx_comments_parent_id ON comments(parent_id)"))


def _ensure_blueprint_image_object_key(sync_conn) -> None:
    inspector = inspect(sync_conn)
    if "blueprint_images" not in inspector.get_table_names():
        return

    columns = {column["name"] for column in inspector.get_columns("blueprint_images")}
    if "object_key" not in columns:
        sync_conn.execute(text("ALTER TABLE blueprint_images ADD COLUMN object_key VARCHAR(500)"))
    sync_conn.execute(text("CREATE INDEX IF NOT EXISTS idx_blueprint_images_object_key ON blueprint_images(object_key)"))


def _ensure_blueprint_image_file_type(sync_conn) -> None:
    inspector = inspect(sync_conn)
    if "blueprint_images" not in inspector.get_table_names():
        return

    columns = {column["name"] for column in inspector.get_columns("blueprint_images")}
    if "file_type" not in columns:
        sync_conn.execute(text("ALTER TABLE blueprint_images ADD COLUMN file_type VARCHAR(10) DEFAULT 'image'"))
