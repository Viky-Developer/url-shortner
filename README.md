# url-shortner

A URL shortening service built with **Go**, using **sqlc + goose** for type-safe database access and migrations, **Redis** for session caching, and **JWT** for stateless authentication.

## Features

- Create short URLs from long URLs (custom or random 10-char codes)
- Redirect short URLs to their original destination
- Click analytics with daily stats, referrer/device/browser breakdowns
- User authentication (register, login, password reset) with JWT + session management
- Multi-device session tracking with device limit enforcement
- Self-service account deletion with a 30-day grace period
- Admin panel: blocked domains/IP ranges, user management, maintenance purges
- Destination health checks on URL creation/update
- Role-based access control (USER / ADMIN)
- Type-safe SQL queries generated with sqlc
- Database migrations managed with goose
- Session caching with Redis

## Architecture

```
Client ──► HTTP Server (cmd/server)
              │
              ├── handler ──► service ──► db (sqlc generated)
              │                                │
              │                                ├── Postgres (goose migrations)
              │                                └── Redis (session cache)
              │
              └── middleware (auth, role, logging, recovery)
```

- **cmd/server** — application entry point
- **internal/handler** — HTTP handlers (auth, URL, admin, account)
- **internal/service** — business logic (auth, URL, admin, account deletion, retention)
- **internal/db** — database layer (migrations + queries + generated code)
- **internal/db/migrations** — goose SQL migrations (schema source of truth)
- **internal/db/queries** — raw SQL queries used by sqlc
- **internal/db/gen** — sqlc-generated type-safe Go code
- **internal/middleware** — auth, role enforcement, logging, content-type
- **internal/apperror** — sentinel errors mapped to HTTP status codes
- **internal/payload** — request/response structs
- **internal/response** — JSON response helpers
- **internal/routes** — route registration on ServeMux
- **internal/validation** — request binding and validation
- **internal/contextutil** — request context helpers
- **internal/enum** — role and status enums
- **internal/graceful** — graceful shutdown
- **internal/utils** — encoding, IP, string utilities
- **external/logger** — structured logging (zap)
- **external/cache** — Redis session cache

## Requirements

- Go 1.26+
- [sqlc](https://sqlc.dev/) (installed via `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`)
- [goose](https://pressly.github.io/goose/) for database migrations
- Docker & Docker Compose (for Postgres and Redis)
- PostgreSQL for data storage
- Redis for session caching (graceful fallback if unavailable)
- [lefthook](https://lefthook.dev/) for git hooks (install via `go install github.com/evilmartians/lefthook@latest`)
- [golangci-lint](https://golangci-lint.run/) for linting

## Git Hooks (lefthook)

Git hooks are managed with [lefthook](https://lefthook.dev/), configured in `lefthook.yml`. Install them once:

```bash
lefthook install
```

### pre-commit

Runs in parallel (up to 4) over staged Go files:

| Check      | What it does                                             |
|------------|----------------------------------------------------------|
| `gofmt`    | Fails if staged `.go` files are unformatted               |
| `goimports`| Fails on import formatting issues (skipped on merges)     |
| `go-vet`   | Runs `go vet ./...`                                       |
| `golangci` | Runs `golangci-lint run ./... --timeout 5m`               |
| `sqlc`     | Runs `sqlc generate` and fails if `internal/db/gen` is stale |
| `build`    | Runs `go build ./...`                                     |

> `sqlc`, `sqlc-dev`, and a Go toolchain must be on `PATH` for the hooks to pass.

### commit-msg

Enforces [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<optional scope>): <subject>
```

Allowed types: `feat, fix, refactor, chore, docs, test, perf, ci, style, build, revert`. Merges are skipped.

### pre-push (branch naming)

Branches must use a context prefix. Pushes are blocked if the name does not match:

| Prefix      | Purpose                   |
|-------------|---------------------------|
| `feat/`     | New features              |
| `refactor/` | Refactoring existing code |
| `bug/`      | Bug fixes                 |
| `fix/`      | Immediate fixes (dev/main)|
| `hotfix/`   | Urgent production fixes   |
| `chore/`    | Maintenance tasks         |

> `main`, `dev`, and `migrations` are allowlisted and can be pushed without a prefix.

Create a properly named branch with the Makefile helper:

```bash
make branch type=feat name=add-login
```

This runs `git checkout -b feat/add-login`.

## Database (sqlc + goose)

Database access is **not** ORM-based. Instead:

- **goose** owns the schema via SQL migrations in `internal/db/migrations`.
- **sqlc** generates type-safe Go code (in `internal/db/gen`) from the raw queries in `internal/db/queries` and the schema in `internal/db/migrations`. Configuration lives in `sqlc.yaml`.

### Generate code (after changing schema or queries)

```bash
sqlc generate
```

### Apply migrations

```bash
goose -dir internal/db/migrations postgres "$DB_DSN" up
```

> Do not edit files in `internal/db/gen` by hand — they are regenerated by sqlc.

## Getting Started

### 1. Start infrastructure with Docker Compose

```bash
make docker-up
```

This brings up:

| Service   | Port  | Default credentials   |
|-----------|-------|-----------------------|
| Postgres  | 5432  | urlshortner / urlshortner123 |
| Redis     | 6379  | (no password)         |

### 2. Apply database migrations

```bash
make migration-up
```

### 3. Run the server

```bash
make run
```

Or with live reload (Air):

```bash
make dev
```

## Configuration

Configuration is handled via environment variables (loaded from `.env`). See `internal/config` for all supported keys:

| Variable | Default | Description |
|---|---|---|
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `urlshortner` | Database user |
| `DB_PASSWORD` | `urlshortner123` | Database password |
| `DB_NAME` | `urlshortner` | Database name |
| `DB_SSLMODE` | `disable` | SSL mode |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `JWT_SECRET_KEY` | — | HMAC signing key for JWT tokens |
| `USER_ID_SECRET_KEY` | — | HMAC key for encoding user display IDs |
| `ACCESS_TOKEN_EXPIRY` | `15` | Access token lifetime (minutes) |
| `REFRESH_TOKEN_EXPIRY` | `7` | Refresh token lifetime (days) |
| `REDIS_HOST` | `localhost` | Redis host |
| `REDIS_PORT` | `6379` | Redis port |
| `REDIS_USERNAME` | `""` | Redis AUTH username |
| `REDIS_PASSWORD` | `""` | Redis AUTH password |
| `REDIS_DB` | `0` | Redis database number |

## API

### Auth (public)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/auth/register` | Register a new user |
| `POST` | `/api/v1/auth/login` | Log in (sends device info in headers) |
| `POST` | `/api/v1/auth/forgot-password` | Reset password with current password |

### Auth (protected — requires access token)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/auth/refresh` | Refresh access token |
| `POST` | `/api/v1/auth/change-password` | Change password |
| `POST` | `/api/v1/auth/logout` | Log out and revoke session |
| `GET` | `/api/v1/auth/sessions` | List active sessions |
| `DELETE` | `/api/v1/auth/sessions/{id}` | Revoke a session |
| `POST` | `/api/v1/auth/sessions/revoke-others` | Revoke all other sessions |
| `POST` | `/api/v1/auth/sessions/revoke-all` | Revoke all sessions |

### URLs (protected)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/shorten` | Create a short URL |
| `GET` | `/api/v1/urls` | List user's URLs (paginated) |
| `GET` | `/api/v1/urls/{id}` | Get URL by ID |
| `PATCH` | `/api/v1/urls/{id}` | Update URL |
| `DELETE` | `/api/v1/urls/{id}` | Soft delete a URL |
| `DELETE` | `/api/v1/urls/{id}/approve` | Confirm hard delete |
| `GET` | `/api/v1/urls/{id}/clicks` | Click log with date filters |
| `GET` | `/api/v1/urls/{id}/analytics` | Daily stats, top referrers, browsers |

### Account (protected)

| Method | Path | Description |
|--------|------|-------------|
| `DELETE` | `/api/v1/account` | Request account deletion (30-day grace) |
| `POST` | `/api/v1/account/cancel-deletion` | Cancel pending deletion |
| `GET` | `/api/v1/account/status` | Check account status |

### Admin (protected — ADMIN role required)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/admin/blocked-domains` | List blocked domains |
| `POST` | `/api/v1/admin/blocked-domains` | Block a domain |
| `DELETE` | `/api/v1/admin/blocked-domains/{id}` | Unblock a domain |
| `GET` | `/api/v1/admin/blocked-ip-ranges` | List blocked IP ranges |
| `POST` | `/api/v1/admin/blocked-ip-ranges` | Block an IP range |
| `DELETE` | `/api/v1/admin/blocked-ip-ranges/{id}` | Unblock an IP range |
| `DELETE` | `/api/v1/admin/users/{id}/soft-delete` | Soft delete a user |
| `DELETE` | `/api/v1/admin/users/{id}/hard-delete` | Hard delete a user |
| `POST` | `/api/v1/admin/maintenance/purge-sessions` | Purge old revoked sessions |
| `POST` | `/api/v1/admin/maintenance/purge-password-history` | Purge old password history |

### Public

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/{shortCode}` | Redirect to original URL |

## Project Structure

```
.
├── cmd/server              # application entry point
├── internal
│   ├── apperror/           # sentinel errors
│   ├── config/             # configuration (env vars)
│   ├── contextutil/        # request context helpers
│   ├── db
│   │   ├── gen/            # sqlc-generated code
│   │   ├── migrations/     # goose SQL migrations
│   │   ├── queries/        # raw SQL queries (sqlc input)
│   │   └── seeds/          # seed data migrations
│   ├── enum/               # role and status enums
│   ├── graceful/           # graceful shutdown
│   ├── handler/            # HTTP handlers (auth, URL, admin, account)
│   ├── middleware/          # auth, role, logging, content-type
│   ├── payload/            # request/response structs
│   ├── response/           # JSON response helpers
│   ├── routes/             # route registration
│   ├── service/            # business logic
│   ├── utils/              # encoding, IP, string utilities
│   └── validation/         # request binding and validation
├── external
│   ├── cache/              # Redis session cache
│   └── logger/             # structured logging (zap)
├── scripts/                # helper scripts
├── sqlc.yaml               # sqlc configuration
├── lefthook.yml            # git hooks config
├── docker-compose.yml      # Postgres + Redis
├── Makefile                # dev commands
└── README.md
```

## License

See [LICENSE](LICENSE).