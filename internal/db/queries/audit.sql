-- name: InsertAuditLog :exec
INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id, metadata)
VALUES ($1, $2, $3, $4, $5);

-- name: ListAuditLogs :many
SELECT id, actor_user_id, action, entity_type, entity_id, metadata, created_at
FROM audit_logs
WHERE ($1 IS NULL OR actor_user_id = $1)
  AND ($2 IS NULL OR action = $2)
  AND ($3 IS NULL OR entity_type = $3)
  AND ($4 IS NULL OR entity_id = $4)
ORDER BY created_at DESC
LIMIT $5 OFFSET $6;

-- name: CountAuditLogs :one
SELECT COUNT(*) FROM audit_logs
WHERE ($1 IS NULL OR actor_user_id = $1)
  AND ($2 IS NULL OR action = $2)
  AND ($3 IS NULL OR entity_type = $3)
  AND ($4 IS NULL OR entity_id = $4);