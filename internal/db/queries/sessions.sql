-- name: CreateSession :one
INSERT INTO sessions (user_id, refresh_token_hash, device_type, device_name, ip_address, user_agent)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, user_id, refresh_token_hash, device_type, device_name, ip_address, user_agent, logged_in_at, last_active_at, session_status;

-- name: GetSessionByRefreshTokenHash :one
SELECT id, user_id, refresh_token_hash, device_type, device_name, ip_address, user_agent, logged_in_at, last_active_at, session_status
FROM sessions
WHERE refresh_token_hash = $1 AND session_status = 1;

-- name: GetSessionByID :one
SELECT id, user_id, refresh_token_hash, device_type, device_name, ip_address, user_agent, logged_in_at, last_active_at, session_status
FROM sessions
WHERE id = $1;

-- name: UpdateSessionLastActive :exec
UPDATE sessions SET last_active_at = NOW() WHERE id = $1 AND session_status = 1;

-- name: RevokeSession :exec
UPDATE sessions SET session_status = 0 WHERE id = $1 AND user_id = $2;

-- name: ListSessionsByUser :many
SELECT id, user_id, refresh_token_hash, device_type, device_name, ip_address, user_agent, logged_in_at, last_active_at, session_status
FROM sessions
WHERE user_id = $1 AND session_status = 1
ORDER BY last_active_at DESC;

-- name: ListActiveSessionsByUser :many
SELECT id, user_id, refresh_token_hash, device_type, device_name, ip_address, user_agent, logged_in_at, last_active_at, session_status
FROM sessions
WHERE user_id = $1 AND session_status = 1
ORDER BY last_active_at ASC;

-- name: PurgeOldRevokedSessions :exec
DELETE FROM sessions
WHERE session_status = 0 AND last_active_at < $1;

-- name: PurgeInactiveSessions :exec
DELETE FROM sessions
WHERE session_status = 1 AND last_active_at < $1;

-- name: CountRevokedSessions :one
SELECT COUNT(*) FROM sessions WHERE session_status = 0;