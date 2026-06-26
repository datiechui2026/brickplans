from contextlib import asynccontextmanager
from pathlib import Path

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.staticfiles import StaticFiles

from app.core.config import get_settings
from app.core.database import get_engine
from app.core.migrations import prepare_database
from app.api import auth
from app.api import blueprints
from app.api import images
from app.api import reports
from app.api import tags
from app.api import users
from app.api import seo
from app.api import stats
from app.api import admin
from app.api import notifications

settings = get_settings()


@asynccontextmanager
async def lifespan(app: FastAPI):
    engine = get_engine()
    await prepare_database(engine)
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


# ── Pure ASGI middleware: force PDF Content-Disposition to inline ──
# BaseHTTPMiddleware breaks StaticFiles streaming, so we use raw ASGI.
class PDFInlineASGIMiddleware:
    """Force Content-Disposition: inline for /uploads/*.pdf so browsers preview instead of downloading."""

    def __init__(self, app):
        self.app = app

    async def __call__(self, scope, receive, send):
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        path = scope.get("path", "")

        async def _send(message):
            if message["type"] == "http.response.start" and path.startswith("/uploads/") and path.lower().endswith(".pdf"):
                headers = dict(message.get("headers", []))
                headers[b"content-disposition"] = b"inline"
                # Ensure correct MIME type
                content_type_key = None
                for k in headers:
                    if k.lower() == b"content-type":
                        content_type_key = k
                        break
                if content_type_key is None:
                    headers[b"content-type"] = b"application/pdf"
                message["headers"] = list(headers.items())
            await send(message)

        await self.app(scope, receive, _send)


app.add_middleware(PDFInlineASGIMiddleware)

app.include_router(auth.router)
app.include_router(blueprints.router)
app.include_router(images.router)
app.include_router(reports.router)
app.include_router(tags.router)
app.include_router(users.router)
app.include_router(seo.router)
app.include_router(stats.router)
app.include_router(admin.router)
app.include_router(notifications.router)

# Mount uploads directory for serving uploaded images
uploads_path = Path(__file__).resolve().parent.parent / "uploads"
uploads_path.mkdir(parents=True, exist_ok=True)
app.mount("/uploads", StaticFiles(directory=str(uploads_path)), name="uploads")


@app.get("/api/health")
async def health():
    return {"status": "ok", "version": "0.1.0"}
