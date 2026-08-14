-- +goose Up
ALTER TABLE urls ADD COLUMN deleted_at TIMESTAMPTZ NULL;

-- +goose Down
ALTER TABLE urls DROP COLUMN deleted_at;
