-- +goose Up
-- +goose StatementBegin
ALTER TABLE products ADD COLUMN bundles JSONB;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE products DROP COLUMN bundles;
-- +goose StatementEnd
