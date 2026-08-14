ALTER TABLE users ADD COLUMN IF NOT EXISTS affiliate_id VARCHAR(20);
CREATE INDEX IF NOT EXISTS idx_users_affiliate_id ON users(affiliate_id);
