# URL Shortener Microservice

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://golang.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16+-336791?style=flat&logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-7+-DC382D?style=flat&logo=redis&logoColor=white)](https://redis.io/)
[![RabbitMQ](https://img.shields.io/badge/RabbitMQ-3+-FF6600?style=flat&logo=rabbitmq&logoColor=white)](https://www.rabbitmq.com/)
[![Prometheus](https://img.shields.io/badge/Prometheus-Monitoring-E6522C?style=flat&logo=prometheus&logoColor=white)](https://prometheus.io/)
[![Grafana](https://img.shields.io/badge/Grafana-Dashboards-F46800?style=flat&logo=grafana&logoColor=white)](https://grafana.com/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A high-performance, resilient, and production-grade **URL Shortening and Link Management Service** written in **Go**. Built using clean architecture, type-safe database queries (**sqlc**), versioned migrations (**goose**), distributed in-memory caching (**Redis**), asynchronous event processing (**RabbitMQ**), and full-stack observability (**Prometheus & Grafana**).

---

## Table of Contents

- [Key Features](#key-features)
- [System Architecture](#system-architecture)
- [Tech Stack](#tech-stack)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Quickstart with Docker Compose](#quickstart-with-docker-compose)
  - [Database Migrations](#database-migrations)
  - [Running the Application](#running-the-application)
- [Configuration Reference](#configuration-reference)
- [API Reference](#api-reference)
  - [Health & Observability](#health--observability)
  - [Redirection (Public)](#redirection-public)
  - [Authentication & Sessions](#authentication--sessions)
  - [URL Management & Analytics](#url-management--analytics)
  - [Account Management](#account-management)
  - [Admin Panel](#admin-panel)
- [Observability & Monitoring](#observability--monitoring)
- [Development & Quality Assurance](#development--quality-assurance)
  - [Makefile Commands](#makefile-commands)
  - [Git Hooks & Commit Guidelines](#git-hooks--commit-guidelines)
- [License](#license)

---

## Key Features

- **Blazing Fast Redirects**: Sub-millisecond response times with **Redis 30-minute TTL caching** (zero database queries on cache hits).
- **Asynchronous Click Analytics**: High-throughput click ingestion via **RabbitMQ** direct exchange (`url.clicks.direct`), routing keys (`url.clicks.route`), and durable queues (`url.clicks`).
- **Resilient Fallback**: Automatically degrades gracefully to synchronous transactional database writes if RabbitMQ or Redis is unavailable or unconfigured.
- **Rich Analytics & Reporting**: Tracks referrers, device types (desktop/mobile/tablet), browsers, and cumulative daily click aggregations.
- **Robust Security**:
  - Domain blacklisting to prevent phishing and malicious URL targets.
  - CIDR-based IP range blocking.
  - Active URL destination health checks before short code creation or updates.
  - Log sanitization to prevent log injection vulnerabilities.
- **Stateless Authentication & Session Management**:
  - JWT Access Token + Refresh Token rotation with multi-device tracking.
  - Device session limits with remote revocation (revoke single, other, or all devices).
  - Self-service account deletion lifecycle with a 30-day grace period.
- **Role-Based Access Control (RBAC)**: Distinct permissions for `USER` and `ADMIN` roles.
- **Production Observability**: Built-in Prometheus metrics exposition (`/metrics`) and auto-provisioned Grafana monitoring dashboards.
- **Type-Safe Persistence**: Pure SQL with 100% type-safe Go code generation using **sqlc** and **goose** migrations.

---

## System Architecture

```text
                            ┌────────────────────────────────────────┐
                            │           Client Requests              │
                            └───────────────────┬────────────────────┘
                                                │
                                                ▼
                            ┌────────────────────────────────────────┐
                            │    HTTP Server (cmd/server:8080)       │
                            │  ├── Auth Middleware (JWT / Sessions)  │
                            │  ├── Role Middleware (User / Admin)    │
                            │  └── Metrics & Structured Logging      │
                            └─────────┬───────────────────┬──────────┘
                                      │                   │
         [GET /api/v1/{shortCode}]    │                   │   [Other API Endpoints]
                                      ▼                   ▼
    ┌────────────────────────────────────────┐     ┌─────────────────────────────┐
    │          URL Service (Redirect)        │     │  Auth / Admin / Account Svc │
    └───────┬──────────────────────┬─────────┘     └──────────────┬──────────────┘
            │                      │                              │
     [Cache Hit]            [Cache Miss]                          │
            │                      │                              │
            ▼                      ▼                              │
┌───────────────────────┐  ┌────────────────────────────────┐    │
│  Redis (Cache Service)│  │      PostgreSQL (sqlc)         │◄───┘
│  - 30-min TTL         │  │  - Source of Truth Records     │
└───────────────────────┘  └────────────────────────────────┘
            │                      ▲
     (Async Click Event)           │ (Buffered Transactional Writes)
            │                      │
            ▼                      │
┌───────────────────────┐  ┌────────────────────────────────┐
│  RabbitMQ Broker      │─►│  ClickConsumerWorker (Service) │
│  - Exchange & Routing │  │  - Manual ACK / Fair QoS (10)  │
│  - Durable Queue      │  │  - RecordClickTx Atomicity     │
└───────────────────────┘  └────────────────────────────────┘
```

---

## Tech Stack

| Layer | Technology | Purpose |
|---|---|---|
| **Language** | [Go 1.26+](https://go.dev/) | High-concurrency backend microservice |
| **Primary Database** | [PostgreSQL 16](https://www.postgresql.org/) | Relational store for URLs, users, sessions, and analytics |
| **SQL Compiler** | [sqlc](https://sqlc.dev/) | Generates type-safe Go boilerplate from raw SQL queries |
| **Database Migrations** | [goose](https://pressly.github.io/goose/) | Database schema versioning and seeds |
| **In-Memory Cache** | [Redis 7](https://redis.io/) | URL redirect caching and active user session caching |
| **Message Broker** | [RabbitMQ 3](https://www.rabbitmq.com/) | Decoupled asynchronous click event processing |
| **Metrics & Monitoring**| [Prometheus](https://prometheus.io/) | Real-time application and HTTP performance metric scraping |
| **Visualization** | [Grafana](https://grafana.com/) | Auto-provisioned dashboards for service observability |
| **Git Hooks** | [lefthook](https://lefthook.dev/) | Fast pre-commit formatting, linting, and branch validation |
| **Linter** | [golangci-lint](https://golangci-lint.run/) | Static analysis and code quality assurance |

---

## Getting Started

### Prerequisites

Ensure you have the following installed on your machine:
- **Go 1.26+**
- **Docker & Docker Compose**
- **sqlc**: `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`
- **goose**: `go install github.com/pressly/goose/v3/cmd/goose@latest`
- **lefthook**: `go install github.com/evilmartians/lefthook@latest`
- **golangci-lint**: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`

### Quickstart with Docker Compose

1. **Clone the repository**:
   ```bash
   git clone https://github.com/Viky-Developer/url-shortner.git
   cd url-shortner
   ```

2. **Configure environment variables**:
   ```bash
   cp .env.example .env # or customize .env
   ```

3. **Start infrastructure services**:
   ```bash
   make docker-up
   ```
   This spins up:
   - **PostgreSQL** on `localhost:5432`
   - **Redis** on `localhost:6379`
   - **RabbitMQ** (AMQP on `localhost:5672`, Management UI on `http://localhost:15672`)
   - **Prometheus** on `http://localhost:9090`
   - **Grafana** on `http://localhost:3000`

### Database Migrations

Apply database schema migrations and initial seed records:

```bash
make migration-up
make seed-up
```

### Running the Application

Start the server natively:

```bash
make run
```

Or run with live reload using [Air](https://github.com/air-verse/air):

```bash
make dev
```

The service will listen on `http://localhost:8080` (or your configured `SERVER_PORT`).

---

## Configuration Reference

The application is configured through environment variables loaded from `.env`:

| Variable | Default | Description |
|---|---|---|
| `SERVER_BASE_URL` | `http://localhost:8080` | Base URL used to construct short link redirects |
| `LOG_LEVEL` | `info` | Logging verbosity (`debug`, `info`, `warn`, `error`) |
| `LOG_COLOR` | `true` | Enable or disable ANSI colors in console output (`true`, `false`) |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `urlshortner` | PostgreSQL username |
| `DB_PASSWORD` | *(required)* | PostgreSQL password |
| `DB_NAME` | `urlshortner` | Database name |
| `DB_SSLMODE` | `disable` | PostgreSQL SSL mode (`disable`, `require`, etc.) |
| `DB_MAX_OPEN_CONNS` | `25` | Maximum database open connection pool size |
| `DB_MAX_IDLE_CONNS` | `25` | Maximum database idle connections |
| `DB_MAX_LIFETIME` | `5` | Maximum connection lifetime in minutes |
| `JWT_SECRET_KEY` | *(required)* | Secret key for signing HMAC-SHA256 JWT tokens |
| `USER_ID_SECRET_KEY` | *(required)* | Secret key for obfuscating internal user IDs |
| `ACCESS_TOKEN_EXPIRY` | `15` | Access token lifespan (in minutes) |
| `REFRESH_TOKEN_EXPIRY`| `7` | Refresh token lifespan (in days) |
| `REDIS_HOST` | `localhost` | Redis host |
| `REDIS_PORT` | `6379` | Redis port |
| `REDIS_USERNAME` | `""` | Redis authentication username (if required) |
| `REDIS_PASSWORD` | `""` | Redis authentication password (if required) |
| `REDIS_DB` | `0` | Redis logical database index |
| `REDIS_MAX_RETRIES` | `3` | Maximum retry attempts for Redis operations |
| `ENABLE_RABBITMQ` | `true` | Toggle asynchronous RabbitMQ click tracking |
| `RABBITMQ_HOST` | `localhost` | RabbitMQ broker host |
| `RABBITMQ_PORT` | `5672` | RabbitMQ AMQP port |
| `RABBITMQ_USER` | `guest` | RabbitMQ username |
| `RABBITMQ_PASSWORD` | `guest` | RabbitMQ password |
| `RABBITMQ_EXCHANGE_CLICKS` | `url.clicks.direct` | Direct exchange for click events |
| `RABBITMQ_ROUTING_KEY_CLICKS` | `url.clicks.route` | Routing key for click events |
| `RABBITMQ_QUEUE_CLICKS` | `url.clicks` | Durable queue for click consumer |

---

## API Reference

The service exposes RESTful JSON endpoints under the `/api/v1` prefix.

> 📖 **Complete Technical Specification**: For detailed request/response schemas, JSON payloads, pagination parameters, and status codes, see **[docs/api.md](docs/api.md)**.

### Core Endpoints Overview

| Category | Method | Endpoint | Access | Purpose |
|---|---|---|---|---|
| **Health** | `GET` | `/health` | Public | Microservice liveness probe |
| **Metrics** | `GET` | `/metrics` | Public | Prometheus scraping endpoint |
| **Redirect** | `GET` | `/api/v1/{shortCode}` | Public | 302/307 Redirect (Redis cached, RabbitMQ async clicks) |
| **Auth** | `POST` | `/api/v1/auth/register` | Public | Create new user account |
| **Auth** | `POST` | `/api/v1/auth/login` | Public | Authenticate user, issue JWT access + refresh tokens |
| **Auth** | `POST` | `/api/v1/auth/refresh` | Bearer Token | Rotate expired access token |
| **Sessions** | `GET` | `/api/v1/auth/sessions` | Bearer Token | Multi-device session tracking & revocation |
| **URLs** | `POST` | `/api/v1/shorten` | Bearer Token | Shorten URL (with target health checks & domain validation) |
| **URLs** | `GET` | `/api/v1/urls` | Bearer Token | List and search user shortened URLs (paginated) |
| **URLs** | `PATCH` | `/api/v1/urls/{id}` | Bearer Token | Update destination URL & auto-invalidate Redis cache |
| **Analytics** | `GET` | `/api/v1/urls/analytics` | Bearer Token | Aggregate analytics (devices, browsers, referrers) |
| **Account** | `DELETE`| `/api/v1/account` | Bearer Token | Self-service account deletion (30-day grace period) |
| **Admin** | `POST` | `/api/v1/admin/blocked-domains` | Admin Role | Blacklist malicious or phishing domains |
| **Admin** | `POST` | `/api/v1/admin/blocked-ip-ranges`| Admin Role | Block abusive CIDR IP ranges |

👉 *For the full list of all 25+ endpoints with request and response examples, refer to [docs/api.md](docs/api.md).*

---

## Observability & Monitoring

The service exposes Prometheus metrics at `/metrics`.

### Key Metrics Tracked
- `http_requests_total`: Total count of HTTP requests partitioned by HTTP method, route pattern, and status code.
- `http_request_duration_seconds`: Histogram measuring response latency distributions across all handlers.
- `shortener_urls_created_total`: Counter for URLs created.
- `shortener_redirects_served_total`: Counter for short URL redirects served.
- Standard Go runtime metrics (goroutines, heap allocations, GC pause durations).

### Pre-configured Grafana Dashboards
- **URL**: `http://localhost:3000`
- **Default Credentials**: `admin` / `admin`
- Comes pre-configured with automated datasource provisioning and dashboards displaying traffic volume, request latency percentiles, error rates, and system memory.
- **Traffic Generator**: Run `make prometheus-seed` while the server is running to generate synthetic traffic and visualize live metrics.

---

## Development & Quality Assurance

### Makefile Commands

| Command | Action |
|---|---|
| `make help` | View all available Makefile commands |
| `make docker-up` | Start Postgres, Redis, RabbitMQ, Prometheus, and Grafana in background |
| `make docker-down` | Stop and remove running Docker containers |
| `make migration-up` | Apply pending database migrations with goose |
| `make migration-down` | Roll back the most recent migration |
| `make migration-status`| Inspect applied vs pending migrations |
| `make seed-up` | Apply seed data migrations |
| `make sqlc-generate` | Regenerate type-safe Go code from SQL queries (`internal/db/gen`) |
| `make format` | Auto-format Go code using `gofmt` and `goimports` |
| `make lint` | Run `golangci-lint` with all configured linters |
| `make test` | Run full test suite (`go test ./... -count=1`) |
| `make build` | Compile production binary into `bin/url-shortner` |
| `make dev` | Run server with live reloading via `air` |
| `make branch type=<type> issue=<id> name=<name>` | Create properly formatted git branch |

### Git Hooks & Commit Guidelines

Git hooks are managed via [lefthook](https://lefthook.dev/):

1. **Install hooks**:
   ```bash
   make lefthook-install
   ```
2. **Branch Naming**:
   Branches must follow the pattern `<type>/<issue>/<name>`:
   - Allowed types: `feat`, `fix`, `bug`, `refactor`, `chore`, `hotfix`
   - Example: `feat/26/rabbitmq-async-clicks`
3. **Commit Messages**:
   Commits must follow the Conventional Commits format with issue reference and an emoji:
   ```text
   <type>(#<issue>): <emoji> <description>
   ```
   *Example*: `feat(#26): ✨ add logs for rabbitmq publishing and consumption`

---

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.