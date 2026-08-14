-- +goose Up
-- +goose StatementBegin
CREATE TABLE order_items (
  id SERIAL PRIMARY KEY,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp,
  order_id integer,
  product_id integer,
  product_name text,
  product_image text,
  quantity integer,
  price numeric(12, 2),
  FOREIGN KEY(order_id) REFERENCES orders(id) ON DELETE CASCADE,
  FOREIGN KEY(product_id) REFERENCES products(id)
);
CREATE INDEX idx_order_items_deleted_at ON order_items(deleted_at);
CREATE INDEX idx_order_items_order_id ON order_items(order_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE order_items;
-- +goose StatementEnd