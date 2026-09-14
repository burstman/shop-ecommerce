-- +goose Up
CREATE TABLE IF NOT EXISTS ratings (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    order_id BIGINT NOT NULL,
    delivery_stars INTEGER NOT NULL,
    product_stars INTEGER NOT NULL,
    comment TEXT NOT NULL DEFAULT '',
    affiliate_id BIGINT
);
CREATE INDEX IF NOT EXISTS idx_ratings_order_id ON ratings(order_id);
CREATE INDEX IF NOT EXISTS idx_ratings_affiliate_id ON ratings(affiliate_id);

-- +goose Down
DROP TABLE IF EXISTS ratings;