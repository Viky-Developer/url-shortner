-- name: CreateURL :one
INSERT INTO urls (short_code, original_url, is_custom, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetURLByShortCode :one
SELECT *
FROM urls
WHERE short_code = $1
  AND deleted_at IS NULL
  AND (expires_at IS NULL OR expires_at > NOW());

-- name: GetURLByID :one
SELECT *
FROM urls
WHERE id = $1
  AND deleted_at IS NULL;

-- name: ListURLs :many
SELECT *
FROM urls
WHERE deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateURL :one
UPDATE urls
SET original_url = $2, expires_at = $3, updated_at = NOW()
WHERE id = $1
  AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteURL :one
UPDATE urls
SET is_active = FALSE, deleted_at = NOW(), updated_at = NOW()
WHERE id = $1
  AND deleted_at IS NULL
RETURNING *;

-- name: HardDeleteURL :exec
DELETE FROM urls
WHERE id = $1
  AND deleted_at IS NOT NULL;

-- name: CountURLs :one
SELECT COUNT(*)
FROM urls
WHERE deleted_at IS NULL;
