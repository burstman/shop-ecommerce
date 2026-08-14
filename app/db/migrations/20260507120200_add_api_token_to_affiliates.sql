-- +goose Up
-- +goose StatementBegin
ALTER TABLE affiliates ADD COLUMN api_token VARCHAR(64) UNIQUE;
CREATE INDEX idx_affiliates_api_token ON affiliates(api_token);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_affiliates_api_token;
ALTER TABLE affiliates DROP COLUMN IF EXISTS api_token;
-- +goose StatementEnd
