from sqlalchemy import inspect, text
from sqlalchemy.ext.asyncio import AsyncEngine

from app.core.database import Base


async def prepare_database(engine: AsyncEngine) -> None:
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
        await conn.run_sync(_ensure_comment_parent_id)


def _ensure_comment_parent_id(sync_conn) -> None:
    inspector = inspect(sync_conn)
    if "comments" not in inspector.get_table_names():
        return

    columns = {column["name"] for column in inspector.get_columns("comments")}
    if "parent_id" in columns:
        return

    sync_conn.execute(text("ALTER TABLE comments ADD COLUMN parent_id CHAR(32)"))
    sync_conn.execute(text("CREATE INDEX IF NOT EXISTS idx_comments_parent_id ON comments(parent_id)"))
