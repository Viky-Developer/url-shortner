#!/usr/bin/env bash
set -eo pipefail

# Find goose binary: prefer local repo bin, then PATH, then GOPATH
GOOSE_BIN=""
if [ -x "./bin/goose" ]; then
  GOOSE_BIN="./bin/goose"
elif command -v goose >/dev/null 2>&1; then
  GOOSE_BIN="$(command -v goose)"
elif [ -x "$(go env GOPATH 2>/dev/null)/bin/goose" ]; then
  GOOSE_BIN="$(go env GOPATH)/bin/goose"
fi

if [ -z "$GOOSE_BIN" ]; then
  echo "Error: goose binary not found. Please install goose first."
  exit 1
fi

# Load .env if present and DB variables are not yet set
if [ -f ".env" ] && [ -z "$DATABASE_URL" ] && [ -z "$DB_HOST" ]; then
  # shellcheck disable=SC1091
  source .env
fi

# Construct or pick DB connection string
DSN=""
if [ -n "$DATABASE_URL_UNPOOLED" ]; then
  DSN="$DATABASE_URL_UNPOOLED"
elif [ -n "$DATABASE_URL" ]; then
  DSN="$DATABASE_URL"
elif [ -n "$DB_HOST" ] && [ -n "$DB_USER" ] && [ -n "$DB_NAME" ]; then
  PORT="${DB_PORT:-5432}"
  SSL="${DB_SSLMODE:-disable}"
  DSN="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${PORT}/${DB_NAME}?sslmode=${SSL}"
fi

if [ -z "$DSN" ]; then
  echo "Warning: No database connection parameters found (DATABASE_URL or DB_HOST/DB_USER/DB_NAME). Skipping migrations."
  exit 0
fi

# For Neon Postgres: migration tools (goose) require a direct connection, not PgBouncer pooler.
# Automatically replace -pooler in host to use direct compute endpoint for migrations.
DSN_MIGRATE="${DSN//-pooler/}"

# Strip channel_binding if present (not supported by goose lib/pq postgres driver)
DSN_MIGRATE="${DSN_MIGRATE//&channel_binding=require/}"
DSN_MIGRATE="${DSN_MIGRATE//channel_binding=require&/}"
DSN_MIGRATE="${DSN_MIGRATE//?channel_binding=require/?sslmode=require}"

MIGRATIONS_DIR="internal/db/migrations"
SEEDS_DIR="internal/db/seeds"

echo "==> Running schema migrations from ${MIGRATIONS_DIR}..."
"$GOOSE_BIN" -dir "$MIGRATIONS_DIR" postgres "$DSN_MIGRATE" up

echo "==> Running seed migrations from ${SEEDS_DIR}..."
"$GOOSE_BIN" -dir "$SEEDS_DIR" postgres "$DSN_MIGRATE" up

echo "==> Migrations and seeds applied successfully!"

