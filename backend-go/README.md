# BrickPlan backend-go

Go (Gin + GORM + MySQL) rewrite of the Python/FastAPI backend. Same API contract
as `frontend/src/api.js` — the frontend is unchanged. Fixes the security issues
audited in `docs/backend-api-analysis.md`.

## Stack

- Go 1.24, Gin, GORM
- MySQL 8 (production); SQLite (tests only, via `github.com/glebarez/sqlite`)
- JWT (HS256) with `token_version`-based revocation
- bcrypt passwords
- Local filesystem or Tencent COS for uploads

## Layout

```
cmd/server/      # entrypoint
cmd/seed/        # idempotent seed
internal/
  config/        # env loading + startup validation (SECRET_KEY etc.)
  db/            # GORM models + MySQL open
  auth/          # JWT, bcrypt, password policy, auth middleware
  middleware/     # rate limit, security headers, recovery
  storage/       # local + Tencent COS
  upload/        # magic-byte sniffing + image re-encode
  dto/           # request/response structs (public UserOut vs own MeOut)
  handler/       # one file per domain
  router/        # route registration
```

## Develop

1. Have MySQL reachable. Create a DB and user, e.g.:

   ```sql
   CREATE DATABASE brickplans CHARACTER SET utf8mb4;
   CREATE USER 'brickplans'@'localhost' IDENTIFIED BY 'CHANGE_ME';
   GRANT ALL ON brickplans.* TO 'brickplans'@'localhost';
   ```

2. Copy `.env.example` to `.env` and fill in `SECRET_KEY` (≥32 random chars) and
   `MYSQL_DSN`. The server refuses to start with the default `SECRET_KEY`.

3. Run:

   ```bash
   go run ./cmd/server     # serves 127.0.0.1:8100, auto-migrates on startup
   go run ./cmd/seed       # seed sample data (idempotent)
   ```

4. Frontend dev server (`cd ../frontend && npm run dev`) proxies `/api` → 127.0.0.1:8100.

## Test

```bash
go test ./...            # uses SQLite in-memory, no MySQL needed
go test ./... -v -count=1
```

The handler tests cover the security fixes: password policy, token revocation on
password change, refresh endpoint, unpublished-blueprint authorization, private
favorites, no auto-unpublish on reports, avatar upload rejecting `.html`, PNG
stored as `.jpg`, and email/is_admin omitted from public responses.

## Build

```bash
go build -o brickplans-backend ./cmd/server
./brickplans-backend
```

## Production

See `docs/deployment-go.md` for MySQL provisioning, systemd unit, nginx, and
security headers.
