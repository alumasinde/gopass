-- 0009: durable notifications, platform administration, and tenant provisioning metadata.
-- Existing migrations 0001-0008 remain unchanged. Dedicated DBs are metadata-managed
-- but cannot be marked READY until a dedicated tenant router/migration runner is enabled.
SET @s := (SELECT IF(COUNT(*)=0, 'ALTER TABLE organizations ADD COLUMN lifecycle_status VARCHAR(24) NOT NULL DEFAULT ''ACTIVE'' AFTER is_active', 'SELECT 1') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='organizations' AND column_name='lifecycle_status'); PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;
SET @s := (SELECT IF(COUNT(*)=0, 'ALTER TABLE organizations ADD COLUMN database_mode VARCHAR(24) NOT NULL DEFAULT ''SHARED'' AFTER lifecycle_status', 'SELECT 1') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='organizations' AND column_name='database_mode'); PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;
SET @s := (SELECT IF(COUNT(*)=0, 'ALTER TABLE organizations ADD COLUMN provisioning_status VARCHAR(24) NOT NULL DEFAULT ''READY'' AFTER database_mode', 'SELECT 1') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='organizations' AND column_name='provisioning_status'); PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;
SET @s := (SELECT IF(COUNT(*)=0, 'ALTER TABLE organizations ADD COLUMN database_config_ciphertext TEXT NULL AFTER provisioning_status', 'SELECT 1') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='organizations' AND column_name='database_config_ciphertext'); PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;
SET @s := (SELECT IF(COUNT(*)=0, 'ALTER TABLE organizations ADD COLUMN provisioning_error VARCHAR(500) NULL AFTER database_config_ciphertext', 'SELECT 1') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='organizations' AND column_name='provisioning_error'); PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;
SET @s := (SELECT IF(COUNT(*)=0, 'ALTER TABLE organizations ADD COLUMN provisioned_at DATETIME NULL AFTER provisioning_error', 'SELECT 1') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='organizations' AND column_name='provisioned_at'); PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;
SET @s := (SELECT IF(COUNT(*)=0, 'ALTER TABLE organizations ADD COLUMN suspended_at DATETIME NULL AFTER provisioned_at', 'SELECT 1') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='organizations' AND column_name='suspended_at'); PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;

CREATE TABLE IF NOT EXISTS platform_users (
 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
 email VARCHAR(255) NOT NULL UNIQUE,
 password_hash VARCHAR(255) NOT NULL,
 display_name VARCHAR(150) NOT NULL,
 is_active BOOLEAN NOT NULL DEFAULT TRUE,
 last_login_at DATETIME NULL,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS platform_sessions (
 session_id CHAR(32) PRIMARY KEY,
 platform_user_id BIGINT UNSIGNED NOT NULL,
 expires_at DATETIME NOT NULL,
 revoked_at DATETIME NULL,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL,
 KEY idx_platform_sessions_user(platform_user_id, revoked_at, expires_at),
 CONSTRAINT fk_platform_sessions_user FOREIGN KEY(platform_user_id) REFERENCES platform_users(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS platform_audit_logs (
 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
 actor_platform_user_id BIGINT UNSIGNED NULL,
 action VARCHAR(120) NOT NULL,
 resource_type VARCHAR(80) NOT NULL,
 resource_id BIGINT UNSIGNED NULL,
 old_values JSON NULL,
 new_values JSON NULL,
 created_at DATETIME NOT NULL,
 KEY idx_platform_audit_created(created_at),
 KEY idx_platform_audit_resource(resource_type,resource_id),
 CONSTRAINT fk_platform_audit_actor FOREIGN KEY(actor_platform_user_id) REFERENCES platform_users(id) ON DELETE SET NULL
);
CREATE TABLE IF NOT EXISTS notifications (
 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
 organization_id BIGINT UNSIGNED NOT NULL,
 event_type VARCHAR(100) NOT NULL,
 channel VARCHAR(20) NOT NULL DEFAULT 'EMAIL',
 recipient VARCHAR(320) NOT NULL,
 subject VARCHAR(255) NOT NULL,
 payload JSON NOT NULL,
 status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
 attempts INT UNSIGNED NOT NULL DEFAULT 0,
 max_attempts INT UNSIGNED NOT NULL DEFAULT 8,
 next_attempt_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 claimed_at DATETIME NULL,
 delivered_at DATETIME NULL,
 last_error VARCHAR(1000) NULL,
 idempotency_key VARCHAR(190) NULL,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL,
 UNIQUE KEY uq_notifications_org_idempotency(organization_id,idempotency_key),
 KEY idx_notifications_due(status,next_attempt_at,id),
 KEY idx_notifications_org_created(organization_id,created_at),
 CONSTRAINT fk_notifications_org FOREIGN KEY(organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

INSERT IGNORE INTO permissions(code,name,description,module,action,is_system,created_at) VALUES
 ('notifications.view','Notifications View','View delivery status for the current organization','notifications','view',1,UTC_TIMESTAMP()),
 ('notifications.retry','Notifications Retry','Retry dead-letter notification deliveries','notifications','retry',1,UTC_TIMESTAMP());
INSERT IGNORE INTO role_permissions(role_id,permission_id) SELECT r.id,p.id FROM roles r JOIN permissions p ON p.code IN ('notifications.view','notifications.retry') WHERE r.code='organization_admin';
