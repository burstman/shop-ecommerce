-- +goose Up
ALTER TABLE chat_sessions ADD COLUMN channel VARCHAR(10) DEFAULT 'web';
ALTER TABLE chat_sessions ADD COLUMN phone VARCHAR(20) DEFAULT '';

-- +goose Down
ALTER TABLE chat_sessions DROP COLUMN IF EXISTS channel;
ALTER TABLE chat_sessions DROP COLUMN IF EXISTS phone;
