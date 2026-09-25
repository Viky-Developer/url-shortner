-- name: GetOAuthUser :one
SELECT u.id, u.email, u.display_user_id, u.display_user_name, u.role, u.status
FROM oauth_accounts oa
JOIN users u ON u.id = oa.user_id
WHERE oa.provider = $1
  AND oa.provider_subject = $2
  AND u.deleted_at IS NULL;

-- name: CreateOAuthAccount :exec
INSERT INTO oauth_accounts (user_id, provider, provider_subject, provider_email)
VALUES ($1, $2, $3, $4);
