-- +goose Up
-- +goose StatementBegin
ALTER TABLE orders ADD COLUMN whatsapp_blocked BOOLEAN DEFAULT FALSE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE orders DROP COLUMN IF EXISTS whatsapp_blocked;
-- +goose StatementEnd