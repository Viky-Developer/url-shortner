-- +goose Up

ALTER TABLE blocked_ip_ranges ADD COLUMN id BIGSERIAL PRIMARY KEY;

-- +goose Down

ALTER TABLE blocked_ip_ranges DROP COLUMN IF EXISTS id;
