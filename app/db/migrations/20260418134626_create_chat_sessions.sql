-- +goose Up
-- +goose StatementBegin
-- Table for Chat Sessions
CREATE TABLE IF NOT EXISTS chat_sessions (
  id SERIAL PRIMARY KEY,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp,
  identifier text,
  customer_name text,
  is_active BOOLEAN DEFAULT TRUE,
  is_banned BOOLEAN DEFAULT FALSE
);

CREATE UNIQUE INDEX idx_chat_sessions_identifier ON chat_sessions(identifier);
CREATE INDEX idx_chat_sessions_deleted_at ON chat_sessions (deleted_at);

-- Table for Chat Messages
CREATE TABLE IF NOT EXISTS chat_messages (
  id SERIAL PRIMARY KEY,
  chat_session_id integer,
  sender text,
  content text,
  created_at timestamp,
  is_read BOOLEAN DEFAULT FALSE,
  FOREIGN KEY(chat_session_id) REFERENCES chat_sessions(id) ON DELETE CASCADE
);

CREATE INDEX idx_chat_messages_chat_session_id ON chat_messages(chat_session_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS chat_messages;
DROP TABLE IF EXISTS chat_sessions;

-- +goose StatementEnd