-- 0006_hash_credential_tokens: store hashed credential tokens
-- This migration requires careful handling since we're changing the schema
-- First, add the token_hash column
ALTER TABLE credentials ADD COLUMN token_hash VARCHAR(255) NULL AFTER token;

-- Update existing rows to hash their tokens (if any exist)
UPDATE credentials SET token_hash = SHA2(token, 256) WHERE token_hash IS NULL;

-- Make token_hash NOT NULL and unique
ALTER TABLE credentials MODIFY COLUMN token_hash VARCHAR(255) NOT NULL;
ALTER TABLE credentials ADD UNIQUE KEY uq_credentials_token_hash(token_hash);

-- Drop the old token column (keeping token_hash only)
ALTER TABLE credentials DROP COLUMN token;
