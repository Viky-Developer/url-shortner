-- name: CreateURL :one
INSERT INTO urls (short_code, original_url, is_custom, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetURLByShortCode :one
SELECT *
FROM urls
WHERE short_code = $1
  AND (expires_at IS NULL OR expires_at > NOW());
