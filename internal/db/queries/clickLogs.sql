-- name: CreateClickLog :one
INSERT INTO click_logs (url_id, ip_address, user_agent, referrer)
VALUES ($1, $2, $3, $4)
RETURNING *;
