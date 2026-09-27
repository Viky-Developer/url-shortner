-- name: GetDestinationByHash :one
SELECT id, original_url, destination_health_status, last_health_check FROM destinations WHERE url_hash = $1;

-- name: GetDestinationByID :one
SELECT id, original_url, destination_health_status, last_health_check FROM destinations WHERE id = $1;

-- name: CreateDestination :one
INSERT INTO destinations (original_url, url_hash) VALUES ($1, $2)
RETURNING id, original_url, destination_health_status, last_health_check;
