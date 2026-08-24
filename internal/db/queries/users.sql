-- name: GetUserByEmail :one
SELECT id, email, password_hash, display_user_id, display_user_name, password_changed_at FROM users WHERE email = $1 AND deleted_at IS NULL;

-- name: GetUserByID :one
SELECT id, email, display_user_id, display_user_name, password_changed_at FROM users WHERE id = $1 AND deleted_at IS NULL;

-- name: CreateUser :one
INSERT INTO users (email, password_hash, display_user_id, display_user_name, password_changed_at)
VALUES ($1, $2, $3, $4, NOW())
RETURNING id, email, display_user_id, display_user_name, created_at, password_changed_at;

-- name: UpdateUserDisplayID :one
UPDATE users SET display_user_id = $2 WHERE id = $1 AND deleted_at IS NULL
RETURNING id, email, display_user_id, created_at;

-- name: UpdateUserPassword :one
UPDATE users SET password_hash = $2, password_changed_at = NOW() WHERE id = $1 AND deleted_at IS NULL
RETURNING id, email, password_changed_at;

-- name: AddPasswordHistory :exec
INSERT INTO password_history (user_id, password_hash, ip_address, user_agent)
VALUES ($1, $2, $3, $4);

-- name: GetLastPasswordHistory :one
SELECT password_hash FROM password_history WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1;

-- name: PurgeOldPasswordHistory :exec
DELETE FROM password_history WHERE created_at < $1;

-- name: CountPasswordHistory :one
SELECT COUNT(*) FROM password_history WHERE user_id = $1;

-- name: SoftDeleteUser :exec
UPDATE users SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL;

-- name: HardDeleteUser :exec
DELETE FROM users WHERE id = $1 AND deleted_at IS NOT NULL;
