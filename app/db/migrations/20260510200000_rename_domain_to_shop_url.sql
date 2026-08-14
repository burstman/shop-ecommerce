-- Rename domain column to shop_url in affiliates table (only if it still exists)
-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'affiliates' AND column_name = 'domain') THEN
        ALTER TABLE affiliates RENAME COLUMN domain TO shop_url;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'affiliates' AND column_name = 'shop_url') THEN
        ALTER TABLE affiliates RENAME COLUMN shop_url TO domain;
    END IF;
END $$;
-- +goose StatementEnd
