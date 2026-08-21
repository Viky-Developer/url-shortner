-- name: ListBlockedIPRanges :many
SELECT id, cidr, description FROM blocked_ip_ranges ORDER BY id DESC;

-- name: CreateBlockedIPRange :one
INSERT INTO blocked_ip_ranges (cidr, description)
VALUES ($1, $2)
RETURNING id, cidr, description;

-- name: DeleteBlockedIPRange :exec
DELETE FROM blocked_ip_ranges WHERE id = $1;
