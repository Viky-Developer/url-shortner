-- name: InsertAuditLog :exec
INSERT INTO admin_audit_log (admin_id, action, target_type, target_id, details)
VALUES ($1, $2, $3, $4, $5);
