-- +goose Up
ALTER TABLE affiliates DROP CONSTRAINT IF EXISTS uniq_affiliates_shop_url;
ALTER TABLE affiliates ADD CONSTRAINT uniq_affiliates_shop_url UNIQUE (shop_url);

-- +goose Down
ALTER TABLE affiliates DROP CONSTRAINT IF EXISTS uniq_affiliates_shop_url;
