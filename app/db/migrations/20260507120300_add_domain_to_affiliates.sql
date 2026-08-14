-- +goose Up
-- +goose StatementBegin
ALTER TABLE affiliates ADD COLUMN shop_url TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE affiliates DROP COLUMN shop_url;
-- +goose StatementEnd
