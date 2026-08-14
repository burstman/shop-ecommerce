-- +goose Up
-- +goose StatementBegin
CREATE TABLE products (
  id SERIAL PRIMARY KEY,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp,
  name text,
  price numeric(12, 2),
  image text,
  category text,
  stock integer DEFAULT 0,
  promotion_price numeric(12, 2) DEFAULT 0,
  description text
);
CREATE INDEX idx_products_deleted_at ON products(deleted_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE products;
-- +goose StatementEnd
