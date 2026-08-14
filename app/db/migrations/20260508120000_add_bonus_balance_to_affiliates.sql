-- +goose Up
-- +goose StatementBegin
ALTER TABLE affiliates ADD COLUMN balance NUMERIC(12,2) NOT NULL DEFAULT 100.00;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE affiliates DROP COLUMN IF EXISTS balance;
-- +goose StatementEnd
