-- +goose Up
ALTER TABLE products ADD COLUMN bundles_enabled BOOLEAN DEFAULT TRUE;

-- +goose Down
ALTER TABLE products DROP COLUMN bundles_enabled;
