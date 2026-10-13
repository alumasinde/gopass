-- 0010_schema_migration_checksums: record SHA-256 checksums for migrations applied by upgraded runners.
-- Existing rows remain NULL because older releases did not record checksums.
SET @gopass_migration_checksum_sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE schema_migrations ADD COLUMN checksum CHAR(64) NULL AFTER name',
    'SELECT 1'
  )
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'schema_migrations'
    AND column_name = 'checksum'
);
PREPARE gopass_migration_checksum_stmt FROM @gopass_migration_checksum_sql;
EXECUTE gopass_migration_checksum_stmt;
DEALLOCATE PREPARE gopass_migration_checksum_stmt;

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
  UNIQUE KEY uq_platform_migration_runs_active_key (active_key),
  KEY idx_platform_migration_runs_created (created_at),
  KEY idx_platform_migration_runs_status (status)
) ENGINE=InnoDB;
