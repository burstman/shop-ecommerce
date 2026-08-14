-- +goose Up
-- +goose StatementBegin
ALTER TABLE translations ADD COLUMN deleted_at TIMESTAMP WITH TIME ZONE;
CREATE INDEX idx_translations_deleted_at ON translations (deleted_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_translations_deleted_at;
ALTER TABLE translations DROP COLUMN IF EXISTS deleted_at;
-- +goose StatementEnd
