-- +goose Up
-- +goose StatementBegin
CREATE TABLE settings (
  id SERIAL PRIMARY KEY,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp,
  key text UNIQUE,
  value JSONB
);
CREATE INDEX idx_settings_deleted_at ON settings(deleted_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE settings;
-- +goose StatementEnd
