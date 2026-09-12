-- +goose Up
ALTER TABLE orders ADD COLUMN IF NOT EXISTS mescolis_driver_name TEXT;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS mescolis_driver_phone VARCHAR(32);
ALTER TABLE orders ADD COLUMN IF NOT EXISTS in_transit_notified_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE orders DROP COLUMN IF EXISTS in_transit_notified_at;
ALTER TABLE orders DROP COLUMN IF EXISTS mescolis_driver_phone;
ALTER TABLE orders DROP COLUMN IF EXISTS mescolis_driver_name;