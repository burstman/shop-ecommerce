-- +goose Up
-- +goose StatementBegin
ALTER TABLE products ADD COLUMN images JSONB DEFAULT '[]'::JSONB;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE products DROP COLUMN images;
-- +goose StatementEnd
