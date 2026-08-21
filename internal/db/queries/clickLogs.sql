-- name: CreateClickLog :one
INSERT INTO click_logs (url_id, ip_address, user_agent, referrer, browser, device_type)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListClickLogsByURL :many
SELECT
  id, url_id, clicked_at, ip_address, user_agent, referrer, browser, device_type
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

-- name: TopBrowsersByURL :many
SELECT
  COALESCE(browser, 'Unknown') AS browser,
  COUNT(*) AS count
FROM click_logs
WHERE url_id = $1
GROUP BY browser
ORDER BY count DESC
LIMIT $2;

-- name: TopDeviceTypesByURL :many
SELECT
  COALESCE(device_type, 'Unknown') AS device_type,
  COUNT(*) AS count
FROM click_logs
WHERE url_id = $1
GROUP BY device_type
ORDER BY count DESC
LIMIT $2;

-- name: UpsertDailyStats :exec
INSERT INTO daily_url_stats (url_id, stat_date, total_clicks)
VALUES ($1, $2, $3)
ON CONFLICT (url_id, stat_date) DO UPDATE
SET total_clicks = daily_url_stats.total_clicks + EXCLUDED.total_clicks;

-- name: RefreshDailyStats :exec
INSERT INTO daily_url_stats (url_id, stat_date, total_clicks)
SELECT
  url_id,
  DATE(clicked_at) AS stat_date,
  COUNT(*) AS total_clicks
FROM click_logs
WHERE clicked_at >= $1 AND clicked_at < $2
GROUP BY url_id, DATE(clicked_at)
ON CONFLICT (url_id, stat_date) DO UPDATE
SET total_clicks = EXCLUDED.total_clicks;

-- name: GetDailyStatsByURL :many
SELECT stat_date, total_clicks
FROM daily_url_stats
WHERE url_id = $1
  AND ($2::date IS NULL OR stat_date >= $2)
  AND ($3::date IS NULL OR stat_date <= $3)
ORDER BY stat_date ASC;
