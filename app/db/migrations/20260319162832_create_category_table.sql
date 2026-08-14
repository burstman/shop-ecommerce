-- +goose Up
-- +goose StatementBegin
CREATE TABLE categories (
  id SERIAL PRIMARY KEY,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp,
  name text,
  slug text,
  parent_id integer,
  position integer DEFAULT 0,
  is_locked BOOLEAN DEFAULT FALSE,
  FOREIGN KEY(parent_id) REFERENCES categories(id) ON DELETE CASCADE
);
CREATE INDEX idx_categories_deleted_at ON categories(deleted_at);
CREATE INDEX idx_categories_parent_id ON categories(parent_id);

INSERT INTO categories (name, slug, position, is_locked, created_at, updated_at) VALUES ('Home', 'home', 0, TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

CREATE TABLE product_categories (
  product_id integer,
  category_id integer,
  PRIMARY KEY (product_id, category_id),
  FOREIGN KEY(product_id) REFERENCES products(id) ON DELETE CASCADE,
  FOREIGN KEY(category_id) REFERENCES categories(id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE product_categories;
DROP TABLE categories;
-- +goose StatementEnd
