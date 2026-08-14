-- +migrate Up
ALTER TABLE affiliates ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP;

-- +migrate Down
ALTER TABLE affiliates DROP COLUMN IF EXISTS expires_at;
