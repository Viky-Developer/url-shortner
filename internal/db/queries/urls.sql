-- name: CreateURL :one
INSERT INTO urls (user_id, short_code, destination_id, title, description, is_custom, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetURLByShortCode :one
SELECT
  urls.id, urls.user_id, urls.short_code, urls.destination_id,
  urls.title, urls.description, urls.is_custom, urls.is_safe,
  urls.click_count, urls.expires_at, urls.is_active,
  urls.last_accessed_at, urls.destination_status, urls.last_health_check,
  urls.created_at, urls.updated_at, urls.deleted_at,
  destinations.original_url
FROM urls
JOIN destinations ON urls.destination_id = destinations.id
WHERE urls.short_code = $1
  AND urls.deleted_at IS NULL
  AND urls.is_active = TRUE
  AND urls.is_safe = TRUE;

-- name: GetURLByID :one
SELECT
  urls.id, urls.user_id, urls.short_code, urls.destination_id,
  urls.title, urls.description, urls.is_custom, urls.is_safe,
  urls.click_count, urls.expires_at, urls.is_active,
  urls.last_accessed_at, urls.destination_status, urls.last_health_check,
  urls.created_at, urls.updated_at, urls.deleted_at,
  destinations.original_url
FROM urls
JOIN destinations ON urls.destination_id = destinations.id
WHERE urls.id = $1
  AND urls.user_id = $2
  AND urls.deleted_at IS NULL;

-- name: ListURLs :many
SELECT
  urls.id, urls.user_id, urls.short_code, urls.destination_id,
  urls.title, urls.description, urls.is_custom, urls.is_safe,
  urls.click_count, urls.expires_at, urls.is_active,
  urls.last_accessed_at, urls.destination_status, urls.last_health_check,
  urls.created_at, urls.updated_at, urls.deleted_at,
  destinations.original_url
FROM urls
JOIN destinations ON urls.destination_id = destinations.id
WHERE urls.user_id = $1
  AND urls.deleted_at IS NULL
ORDER BY urls.created_at DESC
LIMIT $2 OFFSET $3;

-- name: UpdateURL :one
UPDATE urls
SET destination_id = $3,
    title = COALESCE(sqlc.narg('title'), title),
    description = COALESCE(sqlc.narg('description'), description),
    expires_at = sqlc.narg('expires_at'),
    is_active = COALESCE(sqlc.narg('is_active'), is_active),
    updated_at = NOW()
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

-- name: GetURLByShortCodeForUpdate :one
SELECT
  urls.id, urls.user_id, urls.short_code, urls.destination_id,
  urls.title, urls.description, urls.is_custom, urls.is_safe,
  urls.click_count, urls.expires_at, urls.is_active,
  urls.last_accessed_at, urls.destination_status, urls.last_health_check,
  urls.created_at, urls.updated_at, urls.deleted_at,
  destinations.original_url
FROM urls
JOIN destinations ON urls.destination_id = destinations.id
WHERE urls.short_code = $1
  AND urls.deleted_at IS NULL
  AND urls.is_active = TRUE
  AND urls.is_safe = TRUE
FOR UPDATE OF urls;

-- name: IncrementURLClick :exec
UPDATE urls
SET click_count = COALESCE(click_count, 0) + 1,
  updated_at = NOW()
WHERE id = $1;

-- name: UpdateURLHealthStatus :one
UPDATE urls
SET destination_status = $2,
    destination_http_code = $3,
    last_health_check = $4,
    last_accessed_at = $5,
    updated_at = NOW()
WHERE id = $1
RETURNING *;
