-- +goose Up
-- +goose StatementBegin
ALTER TABLE affiliates ADD COLUMN IF NOT EXISTS api_key VARCHAR(128) UNIQUE;
CREATE INDEX IF NOT EXISTS idx_affiliates_api_key ON affiliates(api_key);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_affiliates_api_key;
ALTER TABLE affiliates DROP COLUMN IF EXISTS api_key;
-- +goose StatementEnd
