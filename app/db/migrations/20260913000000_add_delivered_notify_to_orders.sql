-- +goose Up
ALTER TABLE orders ADD COLUMN IF NOT EXISTS delivered_notified_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE orders DROP COLUMN IF EXISTS delivered_notified_at;