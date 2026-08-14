-- Repair affiliates table columns to match the Go model:
--   * rename legacy api_token column to api_key
--   * add authorized_email column used by setup flow

-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'affiliates' AND column_name = 'api_token') THEN
        ALTER TABLE affiliates RENAME COLUMN api_token TO api_key;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE affiliates ADD COLUMN IF NOT EXISTS authorized_email VARCHAR(255);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'affiliates' AND column_name = 'api_key') THEN
        ALTER TABLE affiliates RENAME COLUMN api_key TO api_token;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE affiliates DROP COLUMN IF EXISTS authorized_email;
