-- Rename domain column to shop_url in affiliates table
-- +migrate Up
ALTER TABLE affiliates RENAME COLUMN domain TO shop_url;

-- +migrate Down
ALTER TABLE affiliates RENAME COLUMN shop_url TO domain;
