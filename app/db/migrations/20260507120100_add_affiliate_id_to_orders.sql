-- +goose Up
ALTER TABLE orders ADD COLUMN affiliate_id INT REFERENCES affiliates(id);
CREATE INDEX idx_orders_affiliate_id ON orders(affiliate_id);

-- +goose Down
DROP INDEX IF EXISTS idx_orders_affiliate_id;
ALTER TABLE orders DROP COLUMN IF EXISTS affiliate_id;
