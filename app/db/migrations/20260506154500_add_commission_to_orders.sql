-- +goose Up
-- +goose StatementBegin
ALTER TABLE orders ADD COLUMN platform_commission NUMERIC(12,2) DEFAULT 0;
ALTER TABLE orders ADD COLUMN commission_status VARCHAR(20) DEFAULT 'pending'; -- pending, paid, cancelled
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE orders DROP COLUMN IF EXISTS platform_commission;
ALTER TABLE orders DROP COLUMN IF EXISTS commission_status;
-- +goose StatementEnd
