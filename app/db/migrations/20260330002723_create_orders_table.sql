-- +goose Up
-- +goose StatementBegin
CREATE TABLE orders (
  id SERIAL PRIMARY KEY,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp,
  first_name text,
  last_name text,
  email text,
  address text,
  city text,
  phone text,
  total numeric(12, 2),
  status text DEFAULT 'pending'
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE orders;
-- +goose StatementEnd
