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
- Observability with Prometheus metrics and Grafana dashboards

### Roadmap

- Async link analytics / click tracking via RabbitMQ

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

> Additional services (RabbitMQ) are planned — see [Roadmap](#roadmap-1). Prometheus and Grafana are part of the [Observability](#observability-grafana--prometheus) stack.

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

---

## Roadmap

The following features are planned for future implementation.

### RabbitMQ

RabbitMQ will be used for async event processing, such as recording click count per short link. The service will publish messages (e.g. click events) to a queue, and consumers will handle the analytics.

```yaml
# docker-compose.yml (planned)
rabbitmq:
  image: rabbitmq:3-management
  ports:
    - "5672:5672"
    - "15672:15672"
```

| Service   | URL                    | Default credentials |
|-----------|------------------------|---------------------|
| RabbitMQ  | http://localhost:15672 | guest / guest       |

### Observability (Grafana + Prometheus)

The service exposes a Prometheus metrics endpoint at `/metrics` (`external/metrics` wraps the Prometheus client library, following the integration-layer conventions of `external/cache` and `external/logger`).

- **Metrics collected:**
  - HTTP request rate, latency, and status distribution (`http_requests_total`, `http_request_duration_seconds`) via a metrics middleware in `internal/middleware`
  - Business counters for URLs created and redirects served (`shortener_urls_created_total`, `shortener_redirects_served_total`)
- **Prometheus** scrapes the service metrics endpoint (scrape config: `deploy/prometheus/prometheus.yml`).
- **Grafana** connects to Prometheus as a data source and auto-provisions the "URL Shortener Monitoring" dashboard (`deploy/grafana/`) with:
  - Request rate and latency panels
  - Total URLs created / redirects served
  - Go runtime panels (goroutines, heap)

The Prometheus and Grafana services are defined in `docker-compose.yml` and start together with `make docker-up`:

```yaml
prometheus:
  image: prom/prometheus
  ports:
    - "9090:9090"

grafana:
  image: grafana/grafana
  ports:
    - "3000:3000"
  environment:
    GF_SECURITY_ADMIN_USER: admin
    GF_SECURITY_ADMIN_PASSWORD: admin
```

| Service   | URL                    | Default credentials |
|-----------|------------------------|---------------------|
| Prometheus| http://localhost:9090   | -                   |
| Grafana   | http://localhost:3000   | admin / admin       |