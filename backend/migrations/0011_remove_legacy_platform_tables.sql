-- 0011: Platform Admin data now lives in PLATFORM_DB_DSN. This migration removes
-- only the obsolete platform-only tables from the tenant data plane. For an
-- existing single-database installation, run cmd/platform-import first so the
-- records have been copied to the new platform database before applying this.
DROP TABLE IF EXISTS platform_sessions;
DROP TABLE IF EXISTS platform_audit_logs;
DROP TABLE IF EXISTS platform_users;
DROP TABLE IF EXISTS platform_migration_runs;
