-- name: GetUserByEmail :one
SELECT id, email, display_user_id FROM users WHERE email = $1 AND deleted_at IS NULL;

-- name: CreateUser :one
INSERT INTO users (email, password_hash, display_user_id)
VALUES ($1, $2, $3)
RETURNING id, email, display_user_id, created_at;

-- name: UpdateUserDisplayID :one
UPDATE users SET display_user_id = $2 WHERE id = $1 AND deleted_at IS NULL
RETURNING id, email, display_user_id, created_at;
