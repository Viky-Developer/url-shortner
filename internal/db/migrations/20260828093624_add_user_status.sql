-- +goose Up

-- Add account status column for self-service deletion workflow
ALTER TABLE users
    ADD COLUMN "status" VARCHAR(20) NOT NULL DEFAULT 'ACTIVE';

ALTER TABLE users
    ADD CONSTRAINT "users_status_check"
    CHECK ("status" IN ('ACTIVE', 'PENDING_DELETION', 'DELETED'));

-- Add scheduled deletion timestamp (set when status = PENDING_DELETION)
ALTER TABLE users
    ADD COLUMN "deletion_scheduled_at" TIMESTAMPTZ NULL;

-- Index for background worker: efficiently find accounts due for hard deletion
CREATE INDEX idx_users_deletion_scheduled
    ON users ("deletion_scheduled_at")
    WHERE "status" = 'PENDING_DELETION';

-- Composite index for login status check
CREATE INDEX idx_users_status_email
    ON users ("email", "status");

-- +goose Down

DROP INDEX IF EXISTS idx_users_status_email;
DROP INDEX IF EXISTS idx_users_deletion_scheduled;
ALTER TABLE users DROP CONSTRAINT IF EXISTS "users_status_check";
ALTER TABLE users DROP COLUMN IF EXISTS "deletion_scheduled_at";
ALTER TABLE users DROP COLUMN IF EXISTS "status";
