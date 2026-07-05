# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

BrickPlans (积木图纸分享社区) — a brick/MOC blueprint-sharing community. MVP is live: auth, blueprint CRUD, multi-image/PDF upload, tags, search, favorites/likes/comments, notifications, admin moderation.

**Current deployed form is SQLite + systemd + nginx** (see README). The `docker/` compose stack (Postgres/Redis/MinIO) and the Postgres defaults in `app/core/config.py` are leftover scaffolding for a future migration — do not assume they reflect production. Real config comes from `backend/.env`.

## Commands

Backend (run from `backend/`, using the systemd venv `backend/.venv`):

```bash
uvicorn app.main:app --reload --host 127.0.0.1 --port 8100   # dev server
python -m pytest -q tests                                     # all tests
python -m pytest -q tests/test_blueprints.py                  # one file
python -m pytest -q tests/test_blueprints.py::TestCreateBlueprint::test_create_blueprint_succeeds  # one test
python seed.py                                                # idempotent seed data
curl http://127.0.0.1:8100/api/health                         # health check
```

Frontend (run from `frontend/`):

```bash
npm run dev      # Vite dev server on :5173, proxies /api → 127.0.0.1:8100
npm run build    # production build → frontend/dist/
node --check src/main.js   # syntax check (what CI runs)
```

Pre-commit baseline (CI enforces these, see `.github/workflows/ci.yml`): backend `pytest -q tests` (Python 3.12) and frontend `node --check src/main.js` + `npm run build` (Node 22). Current baseline: backend 60 passed.

## Backend architecture

FastAPI + async SQLAlchemy 2.0 + Pydantic v2. Everything lives under `backend/app/`:

- `main.py` — app factory, `lifespan` runs `prepare_database()` on startup, registers all routers, mounts `/uploads` as StaticFiles, adds a raw-ASGI `PDFInlineASGIMiddleware`.
- `api/*.py` — one router per domain (`auth`, `blueprints`, `images`, `tags`, `users`, `reports`, `notifications`, `admin`, `seo`, `stats`), each mounted with `/api/...` prefix. Routers import directly from `models` / `schemas` / `services`.
- `models/__init__.py` — **all SQLAlchemy models in one file**. IDs are `Uuid(as_uuid=False)` strings (`new_uuid()`), timestamps are UTC. `Comment` supports one-level replies via self-referential `parent_id`.
- `schemas/__init__.py` — **all Pydantic schemas in one file**.
- `core/database.py` — singleton async engine + `get_db()` dependency (commits on success, rolls back on exception).
- `core/security.py` — bcrypt + JWT (python-jose). `create_access_token` / `create_refresh_token` embed a `type` claim (`access`/`refresh`).
- `core/migrations.py` — **the migration system** (not Alembic, despite `alembic` in requirements). On startup `prepare_database()` runs `Base.metadata.create_all` then idempotent `_ensure_*` helpers that `ALTER TABLE ADD COLUMN` for fields added after launch. **When you add a new column to an existing model, also add an `_ensure_*` helper here** or existing SQLite DBs will break.

### Auth pattern

`api/deps.py` has `get_current_user` (401 on failure) and `get_current_admin` (403). For public endpoints that behave differently when logged in (e.g. blueprint detail showing `is_favorited`), `blueprints.py` defines `_optional_user` which decodes the bearer token but returns `None` instead of raising — reuse this pattern rather than reinventing.

### Storage

`services/storage.py` is a pluggable singleton (`get_storage()`): `LocalStorage` writes to `backend/uploads/` and returns `/uploads/<key>` URLs; `TencentCOSStorage` writes to COS and returns full CDN URLs. Backend is chosen by `STORAGE_BACKEND` env (`local` | `tencent_cos`). Callers persist both `url` (what the frontend uses) and `object_key` (what `delete()` needs) on `BlueprintImage`.

### Image/PDF upload (`api/images.py`)

`_validate_and_compress` sniffs magic bytes (PNG/JPEG/WebP/PDF), enforces 20 MB, and re-encodes images to JPEG via Pillow (resize ≤2048px, quality 80) and compresses PDFs via pypdf. `file_type` (`image` | `pdf`) is stored on `BlueprintImage`. Cover-image selection in the frontend skips PDFs since they can't render as `<img>`.

### Gotcha: middleware vs StaticFiles

`BaseHTTPMiddleware` breaks StaticFiles streaming for `/uploads/*.pdf`, so the PDF-inline header is forced via a raw ASGI middleware. Do not "simplify" it back to `BaseHTTPMiddleware`.

## Frontend architecture

Vite + **vanilla JavaScript** (no framework) in a single `src/main.js` (~2000 lines). The design system is one CSS file (`src/styles/main.css`).

- **Hash-based router**: `navigate(page, params)` sets `window.location.hash`; `hashchange` → `applyState` + `render`. Pages: `home`, `explore`, `detail`, `upload`, `edit`, `user`, `notifications`, `admin`, `privacy`.
- DOM is built with an `h(tag, props, ...children)` helper — there are no templates. `render*` functions (`renderHome`, `renderDetail`, …) rebuild page sections.
- `src/api.js` — the only API client. Auth state lives in `localStorage['bp_auth']`. `request()` **auto-refreshes the access token on 401/403** using the refresh token (with a singleton `refreshPromise` to de-dupe concurrent refreshes), then retries once. `formRequest()` does the same for `FormData` uploads. New endpoints go here.

## Deployment

systemd unit `brickplans-backend` runs uvicorn on `127.0.0.1:8100`; nginx (`brickplans-nginx.conf`) serves `frontend/dist` on `0.0.0.0:8310` and reverse-proxies `/api/` and `/uploads/` to the backend. nginx uses `location ^~ /uploads/` so the static-asset regex doesn't steal uploaded files. Full runbooks: `docs/deployment.md`, `docs/production-initialization.md`, `docs/backup.md`.

Config is via `backend/.env` (gitignored) — most importantly `SECRET_KEY` (must not be the default), `DATABASE_URL` / `DATABASE_URL_SYNC` (SQLite path in production), `CORS_ORIGINS`, and `STORAGE_BACKEND`.

## Conventions

- Commits follow conventional-commit prefixes (`feat:`, `fix:`).
- User-facing strings (errors, labels, seed data) are in Chinese; keep that consistent.
- Runtime data is gitignored: `backend/brickplans.db`, `backend/uploads/`, `backend/.env`, `frontend/dist/`, `frontend/.env.production`.
- Feature PRDs live in `docs/PRD-*.md` — check there for intended behavior before changing a feature.

## Backend-Go (new — Gin + MySQL)

`backend-go/` is the Go (Gin + GORM + MySQL) rewrite that **replaces** `backend/` (Python/FastAPI) as the active backend. The Python `backend/` is kept only for reference/rollback. The Go backend preserves the exact API contract in `frontend/src/api.js` — the frontend is unchanged — and fixes the security issues audited in `docs/backend-api-analysis.md`:

- Startup refuses the default `SECRET_KEY` (must be ≥32 chars); `MYSQL_DSN` required.
- JWT carries a `ver` claim bound to `users.token_version`; changing password bumps it → stateless revocation of old tokens.
- Avatar/image uploads sniff magic bytes and re-encode to JPEG under a forced `.jpg` key (blocks stored XSS via `.html`/`.svg`); decoding failure → 422 (never stores raw bytes). PDFs stored as-is (rely on browser sandbox + cross-origin COS in prod).
- Public responses omit `email`, `is_admin`, and `object_key`; only `/api/auth/me` returns them for the user themselves.
- `GET /api/users/{id}/blueprints` hides unpublished from non-authors; `GET /api/users/{id}/favorites` is owner-only (403 otherwise).
- Reports no longer auto-unpublish (no 3-account griefing); they enter the admin review queue.
- Rate limiting on auth/upload/report endpoints; LIKE wildcards escaped; security headers via nginx.

Commands (from `backend-go/`):

```bash
go run ./cmd/server                # dev server (needs MySQL + .env), auto-migrates on startup
go run ./cmd/seed                  # idempotent seed
go test ./...                      # tests use SQLite in-memory, no MySQL needed
go build -o brickplans-backend ./cmd/server
```

Config via `backend-go/.env` (see `.env.example`). nginx still proxies `/api/` + `/uploads/` to `127.0.0.1:8100`. Full runbook: `docs/deployment-go.md`.

Additional hardening implemented after the initial rewrite:

- **Email verification**: register mails a 24h verify link (SMTP configurable via `SMTP_*`; logs the link if `SMTP_HOST` empty for dev); unverified accounts purged after 24h. `GET /api/auth/verify-email?token=`, `POST /api/auth/verify-email/resend`, `POST /api/auth/logout`.
- **Cookie auth (split token)**: refresh token in an httpOnly cookie (`bp_refresh`, Path=/api/auth, SameSite=Lax, Secure in prod); access token held in JS memory only (never localStorage). Frontend `api.js` uses `credentials: 'include'` and restores session on reload via `/api/auth/refresh`.
- **PDF.js**: frontend renders PDFs to `<canvas>` via PDF.js instead of a native `<iframe>` viewer, so embedded PDF JavaScript never executes in the page origin. Falls back to a download link if PDF.js can't load.
- **`cmd/migrate`**: one-shot importer from legacy `backend/brickplans.db` (SQLite) → MySQL, preserving UUIDs, timestamps, and bcrypt hashes. Idempotent (`--reset` to truncate first).
