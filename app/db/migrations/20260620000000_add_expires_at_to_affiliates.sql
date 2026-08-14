-- +goose Up
ALTER TABLE affiliates ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP;

-- +goose Down
ALTER TABLE affiliates DROP COLUMN IF EXISTS expires_at;
