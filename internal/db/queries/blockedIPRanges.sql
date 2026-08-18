-- name: ListBlockedIPRanges :many
SELECT cidr, description FROM blocked_ip_ranges;
