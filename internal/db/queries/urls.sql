-- name: CreateURL :one
INSERT INTO urls (user_id, short_code, original_url, is_custom, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetURLByShortCode :one
SELECT *
FROM urls
WHERE user_id = $1
  AND short_code = $2
  AND deleted_at IS NULL
  AND (expires_at IS NULL OR expires_at > NOW());

-- name: GetURLByID :one
SELECT *
FROM urls
WHERE id = $1
  AND user_id = $2
  AND deleted_at IS NULL;

-- name: ListURLs :many
SELECT *
FROM urls
WHERE user_id = $1
  AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: UpdateURL :one
UPDATE urls
SET original_url = $3, expires_at = $4, updated_at = NOW()
WHERE id = $1
  AND user_id = $2
  AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteURL :one
UPDATE urls
SET is_active = FALSE, deleted_at = NOW(), updated_at = NOW()
WHERE id = $1
  AND user_id = $2
  AND deleted_at IS NULL
RETURNING *;

-- name: HardDeleteURL :exec
DELETE FROM urls
WHERE id = $1
  AND user_id = $2
  AND deleted_at IS NOT NULL;

-- name: CountURLs :one
SELECT COUNT(*)
FROM urls
WHERE user_id = $1
  AND deleted_at IS NULL;
