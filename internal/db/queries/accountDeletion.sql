-- name: GetUserStatusByID :one
SELECT "id", "status", "deletion_scheduled_at" FROM users WHERE "id" = $1 AND "deleted_at" IS NULL;

-- name: MarkPendingDeletion :exec
UPDATE users
SET "status" = 'PENDING_DELETION',
    "deletion_scheduled_at" = NOW() + INTERVAL '30 days',
    "updated_at" = NOW()
WHERE "id" = $1 AND "status" = 'ACTIVE' AND "deleted_at" IS NULL;

-- name: RestoreAccount :exec
UPDATE users
SET "status" = 'ACTIVE',
    "deletion_scheduled_at" = NULL,
    "updated_at" = NOW()
WHERE "id" = $1 AND "status" = 'PENDING_DELETION' AND "deleted_at" IS NULL;

-- name: GetAccountsDueForDeletion :many
SELECT "id" FROM users
WHERE "status" = 'PENDING_DELETION'
  AND "deletion_scheduled_at" <= NOW()
LIMIT 100;

-- name: HardDeleteUserByID :exec
DELETE FROM users WHERE "id" = $1 AND "status" = 'PENDING_DELETION';
