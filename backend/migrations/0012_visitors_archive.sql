-- Soft archive for visitor records; historical gatepass references remain intact.
SET @gopass_visitors_archived_at_exists = (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'visitors' AND COLUMN_NAME = 'archived_at'
);
SET @gopass_visitors_archive_sql = IF(
  @gopass_visitors_archived_at_exists = 0,
  'ALTER TABLE visitors ADD COLUMN archived_at DATETIME NULL DEFAULT NULL AFTER is_blacklisted',
  'SELECT 1'
);
PREPARE gopass_visitors_archive_stmt FROM @gopass_visitors_archive_sql;
EXECUTE gopass_visitors_archive_stmt;
DEALLOCATE PREPARE gopass_visitors_archive_stmt;
