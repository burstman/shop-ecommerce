-- +goose Up
CREATE TABLE IF NOT EXISTS config (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE IF EXISTS config;
