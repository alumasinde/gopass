-- 0007_revocable_sessions: bind access and refresh tokens to a revocable login session.
-- Existing JWTs have no session ID and are intentionally rejected after deployment.
-- Guard DDL so an interrupted MySQL migration can be safely retried.
SET @s := (SELECT IF(COUNT(*)=0, 'ALTER TABLE refresh_tokens ADD COLUMN session_id CHAR(32) NULL AFTER organization_id', 'SELECT 1') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='refresh_tokens' AND column_name='session_id');
PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;
UPDATE refresh_tokens SET session_id=LEFT(SHA2(CONCAT(token_hash, ':legacy-session'), 256), 32) WHERE session_id IS NULL;
SET @s := (SELECT IF(COUNT(*)>0, 'ALTER TABLE refresh_tokens MODIFY COLUMN session_id CHAR(32) NOT NULL', 'SELECT 1') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='refresh_tokens' AND column_name='session_id' AND is_nullable='YES');
PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;
SET @s := (SELECT IF(COUNT(*)=0, 'CREATE INDEX idx_refresh_session_active ON refresh_tokens(session_id, user_id, organization_id, revoked_at, expires_at)', 'SELECT 1') FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='refresh_tokens' AND index_name='idx_refresh_session_active');
PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;
