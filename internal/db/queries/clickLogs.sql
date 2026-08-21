-- name: CreateClickLog :one
INSERT INTO click_logs (url_id, ip_address, user_agent, referrer)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListClickLogsByURL :many
SELECT
  id, url_id, clicked_at, ip_address, user_agent, referrer
FROM click_logs
WHERE url_id = $1
  AND (sqlc.narg('from')::timestamptz IS NULL OR clicked_at >= sqlc.narg('from'))
  AND (sqlc.narg('to')::timestamptz IS NULL OR clicked_at <= sqlc.narg('to'))
ORDER BY clicked_at DESC
LIMIT $2 OFFSET $3;

-- name: CountClickLogsByURL :one
SELECT COUNT(*)
FROM click_logs
WHERE url_id = $1
  AND (sqlc.narg('from')::timestamptz IS NULL OR clicked_at >= sqlc.narg('from'))
  AND (sqlc.narg('to')::timestamptz IS NULL OR clicked_at <= sqlc.narg('to'));

-- name: ClickStatsByURL :one
SELECT
  COUNT(*) AS total_clicks,
  COUNT(DISTINCT ip_address) AS unique_visitors,
  MIN(clicked_at) AS first_clicked_at,
  MAX(clicked_at) AS last_clicked_at
FROM click_logs
WHERE url_id = $1
  AND (sqlc.narg('from')::timestamptz IS NULL OR clicked_at >= sqlc.narg('from'))
  AND (sqlc.narg('to')::timestamptz IS NULL OR clicked_at <= sqlc.narg('to'));

-- name: TopReferrersByURL :many
SELECT
  COALESCE(referrer, '') AS referrer,
  COUNT(*) AS count
FROM click_logs
WHERE url_id = $1
  AND (sqlc.narg('from')::timestamptz IS NULL OR clicked_at >= sqlc.narg('from'))
  AND (sqlc.narg('to')::timestamptz IS NULL OR clicked_at <= sqlc.narg('to'))
  AND referrer IS NOT NULL
  AND referrer != ''
GROUP BY referrer
ORDER BY count DESC
LIMIT $2;

-- name: ClicksByDateRange :many
SELECT
  DATE(clicked_at) AS date,
  COUNT(*) AS clicks
FROM click_logs
WHERE url_id = $1
  AND clicked_at >= $2
  AND clicked_at <= $3
GROUP BY DATE(clicked_at)
ORDER BY date ASC;
