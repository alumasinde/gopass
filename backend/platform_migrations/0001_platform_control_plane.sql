CREATE TABLE IF NOT EXISTS organizations (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
 name VARCHAR(150) NOT NULL,
 slug VARCHAR(100) NOT NULL UNIQUE,
 is_active BOOLEAN NOT NULL DEFAULT FALSE,
 lifecycle_status VARCHAR(24) NOT NULL DEFAULT 'PROVISIONING',
 database_mode VARCHAR(24) NOT NULL DEFAULT 'SHARED',
 provisioning_status VARCHAR(24) NOT NULL DEFAULT 'PENDING',
 database_config_ciphertext TEXT NULL,
 provisioning_error VARCHAR(500) NULL,
 provisioned_at DATETIME NULL,
 suspended_at DATETIME NULL,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL,
 KEY idx_platform_org_lifecycle(lifecycle_status,provisioning_status),
 KEY idx_platform_org_database_mode(database_mode)
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS tenant_bootstrap_requests (
 organization_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
 first_name VARCHAR(100) NOT NULL,
 last_name VARCHAR(100) NOT NULL,
 email VARCHAR(255) NOT NULL,
 password_hash VARCHAR(255) NOT NULL,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL,
 CONSTRAINT fk_tenant_bootstrap_org FOREIGN KEY(organization_id) REFERENCES organizations(id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS platform_users (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
 email VARCHAR(255) NOT NULL UNIQUE,
 password_hash VARCHAR(255) NOT NULL,
 display_name VARCHAR(150) NOT NULL,
 is_active BOOLEAN NOT NULL DEFAULT TRUE,
 last_login_at DATETIME NULL,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS platform_sessions (
 session_id CHAR(32) NOT NULL PRIMARY KEY,
 platform_user_id BIGINT UNSIGNED NOT NULL,
 expires_at DATETIME NOT NULL,
 revoked_at DATETIME NULL,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL,
 KEY idx_platform_sessions_user(platform_user_id,revoked_at,expires_at),
 CONSTRAINT fk_platform_sessions_user FOREIGN KEY(platform_user_id) REFERENCES platform_users(id) ON DELETE CASCADE
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS platform_audit_logs (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
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
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS platform_migration_runs (
 run_id CHAR(32) NOT NULL PRIMARY KEY,
 requested_by BIGINT UNSIGNED NOT NULL,
 target_database VARCHAR(128) NOT NULL,
 status VARCHAR(20) NOT NULL,
 expected_versions JSON NOT NULL,
 applied_versions JSON NULL,
 error_message TEXT NULL,
 active_key VARCHAR(20) NULL,
 started_at DATETIME NULL,
 finished_at DATETIME NULL,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL,
 UNIQUE KEY uq_platform_migration_runs_active_key(active_key),
 KEY idx_platform_migration_runs_created(created_at),
 KEY idx_platform_migration_runs_status(status)
) ENGINE=InnoDB;
