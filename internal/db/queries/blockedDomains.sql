-- name: GetBlockedDomain :one
SELECT id, domain, reason FROM blocked_domains WHERE domain = $1;

-- name: ListBlockedDomains :many
SELECT id, domain, reason, created_at FROM blocked_domains ORDER BY id DESC;

-- name: CreateBlockedDomain :one
INSERT INTO blocked_domains (domain, reason)
VALUES ($1, $2)
RETURNING id, domain, reason, created_at;

-- name: DeleteBlockedDomain :exec
DELETE FROM blocked_domains WHERE id = $1;
