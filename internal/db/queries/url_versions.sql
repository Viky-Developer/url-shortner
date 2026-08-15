-- name: CreateURLVersion :one
INSERT INTO url_versions (url_id, original_url, version_number)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetLatestURLVersion :one
SELECT version_number
FROM url_versions
WHERE url_id = $1
ORDER BY version_number DESC
LIMIT 1;
