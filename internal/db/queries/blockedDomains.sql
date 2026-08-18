-- name: GetBlockedDomain :one
SELECT id, domain, reason FROM blocked_domains WHERE domain = $1;
