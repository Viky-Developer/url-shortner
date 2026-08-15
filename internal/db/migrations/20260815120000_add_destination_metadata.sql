-- +goose Up
ALTER TABLE destinations
    ADD COLUMN destination_status SMALLINT,
    ADD COLUMN last_health_check TIMESTAMPTZ;

-- +goose Down
ALTER TABLE destinations
    DROP COLUMN IF EXISTS last_health_check,
    DROP COLUMN IF EXISTS destination_status;
