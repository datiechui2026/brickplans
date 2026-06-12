from contextlib import asynccontextmanager
from pathlib import Path

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.staticfiles import StaticFiles

from app.core.config import get_settings
from app.core.database import get_engine, Base
from app.api import auth
from app.api import blueprints
from app.api import images
from app.api import reports
from app.api import tags
from app.api import users
from app.api import seo
from app.api import stats
from app.api import admin

settings = get_settings()


@asynccontextmanager
async def lifespan(app: FastAPI):
    engine = get_engine()
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    yield
    await engine.dispose()


app = FastAPI(
    title=settings.app_name,
    version="0.1.0",
    lifespan=lifespan,
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.cors_origins,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

app.include_router(auth.router)
app.include_router(blueprints.router)
app.include_router(images.router)
app.include_router(reports.router)
app.include_router(tags.router)
app.include_router(users.router)
app.include_router(seo.router)
app.include_router(stats.router)
app.include_router(admin.router)

# Mount uploads directory for serving uploaded images
uploads_path = Path(__file__).resolve().parent.parent / "uploads"
uploads_path.mkdir(parents=True, exist_ok=True)
app.mount("/uploads", StaticFiles(directory=str(uploads_path)), name="uploads")


@app.get("/api/health")
async def health():
    return {"status": "ok", "version": "0.1.0"}
