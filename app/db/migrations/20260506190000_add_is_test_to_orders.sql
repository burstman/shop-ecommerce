-- +goose Up
-- +goose StatementBegin
ALTER TABLE orders ADD COLUMN is_test BOOLEAN DEFAULT FALSE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE orders DROP COLUMN IF EXISTS is_test;
-- +goose StatementEnd
