-- 0013: Identification types, visitor ID fields, visits tracking, and blacklist
-- This migration adds support for visitor identification, visit tracking, and blacklist management

-- Create identification_types table
CREATE TABLE IF NOT EXISTS identification_types (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  organization_id BIGINT UNSIGNED NOT NULL,
  code VARCHAR(50) NOT NULL,
  name VARCHAR(100) NOT NULL,
  description TEXT NULL,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  validation_regex VARCHAR(255) NULL,
  validation_example VARCHAR(100) NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE KEY uq_identification_types_org_code(organization_id, code),
  KEY idx_identification_types_org(organization_id),
  CONSTRAINT fk_identification_types_org FOREIGN KEY(organization_id) REFERENCES organizations(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Update visitors table to add identification fields
SET @s := (SELECT IF(COUNT(*)=0, 'ALTER TABLE visitors ADD COLUMN id_type_id BIGINT UNSIGNED NULL AFTER email', 'SELECT 1') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='visitors' AND column_name='id_type_id');
PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;

SET @s := (SELECT IF(COUNT(*)=0, 'ALTER TABLE visitors ADD COLUMN id_number VARCHAR(100) NULL AFTER id_type_id', 'SELECT 1') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='visitors' AND column_name='id_number');
PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;

SET @s := (SELECT IF(COUNT(*)=0, 'ALTER TABLE visitors ADD COLUMN id_expiry_date DATE NULL AFTER id_number', 'SELECT 1') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='visitors' AND column_name='id_expiry_date');
PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;

SET @s := (SELECT IF(COUNT(*)=0, 'ALTER TABLE visitors ADD CONSTRAINT fk_visitors_id_type FOREIGN KEY(id_type_id) REFERENCES identification_types(id) ON DELETE SET NULL', 'SELECT 1') FROM information_schema.table_constraints WHERE table_schema=DATABASE() AND table_name='visitors' AND constraint_name='fk_visitors_id_type');
PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;

-- Create visits table
CREATE TABLE IF NOT EXISTS visits (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  organization_id BIGINT UNSIGNED NOT NULL,
  visitor_id BIGINT UNSIGNED NOT NULL,
  site_id BIGINT UNSIGNED NOT NULL,
  gate_id BIGINT UNSIGNED NOT NULL,
  direction VARCHAR(10) NOT NULL,
  visit_date DATE NOT NULL,
  check_in_time DATETIME NOT NULL,
  check_out_time DATETIME NULL,
  purpose VARCHAR(255) NULL,
  host_name VARCHAR(255) NULL,
  host_department VARCHAR(255) NULL,
  is_blacklisted BOOLEAN NOT NULL DEFAULT FALSE,
  blacklist_reason VARCHAR(500) NULL,
  notes TEXT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  KEY idx_visits_org(organization_id),
  KEY idx_visits_visitor(visitor_id),
  KEY idx_visits_site(site_id),
  KEY idx_visits_gate(gate_id),
  KEY idx_visits_date(visit_date),
  KEY idx_visits_checkin(check_in_time),
  KEY idx_visits_blacklisted(is_blacklisted),
  CONSTRAINT fk_visits_org FOREIGN KEY(organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
  CONSTRAINT fk_visits_visitor FOREIGN KEY(visitor_id) REFERENCES visitors(id) ON DELETE CASCADE,
  CONSTRAINT fk_visits_site FOREIGN KEY(site_id) REFERENCES sites(id) ON DELETE CASCADE,
  CONSTRAINT fk_visits_gate FOREIGN KEY(gate_id) REFERENCES gates(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Create blacklist table
CREATE TABLE IF NOT EXISTS blacklist (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  organization_id BIGINT UNSIGNED NOT NULL,
  visitor_id BIGINT UNSIGNED NULL,
  id_type_id BIGINT UNSIGNED NULL,
  id_number VARCHAR(100) NULL,
  name VARCHAR(255) NULL,
  reason VARCHAR(500) NOT NULL,
  severity VARCHAR(20) NOT NULL DEFAULT 'MEDIUM',
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  expires_at DATETIME NULL,
  added_by BIGINT UNSIGNED NULL,
  notes TEXT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  KEY idx_blacklist_org(organization_id),
  KEY idx_blacklist_visitor(visitor_id),
  KEY idx_blacklist_id_type(id_type_id),
  KEY idx_blacklist_id_number(id_number),
  KEY idx_blacklist_active(is_active, expires_at),
  CONSTRAINT fk_blacklist_org FOREIGN KEY(organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
  CONSTRAINT fk_blacklist_visitor FOREIGN KEY(visitor_id) REFERENCES visitors(id) ON DELETE SET NULL,
  CONSTRAINT fk_blacklist_id_type FOREIGN KEY(id_type_id) REFERENCES identification_types(id) ON DELETE SET NULL,
  CONSTRAINT fk_blacklist_added_by FOREIGN KEY(added_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Insert default identification types for existing organizations
INSERT IGNORE INTO identification_types (organization_id, code, name, description, is_active, is_default, validation_regex, validation_example, created_at, updated_at)
SELECT 
  o.id,
  'NATIONAL_ID',
  'National ID',
  'Government-issued national identification document',
  TRUE,
  TRUE,
  '^[A-Z0-9]{8,20}$',
  'A123456789',
  UTC_TIMESTAMP(),
  UTC_TIMESTAMP()
FROM organizations o
WHERE NOT EXISTS (
  SELECT 1 FROM identification_types it 
  WHERE it.organization_id = o.id AND it.code = 'NATIONAL_ID'
);

INSERT IGNORE INTO identification_types (organization_id, code, name, description, is_active, is_default, validation_regex, validation_example, created_at, updated_at)
SELECT 
  o.id,
  'PASSPORT',
  'Passport',
  'International travel document',
  TRUE,
  FALSE,
  '^[A-Z0-9]{6,12}$',
  'AB1234567',
  UTC_TIMESTAMP(),
  UTC_TIMESTAMP()
FROM organizations o
WHERE NOT EXISTS (
  SELECT 1 FROM identification_types it 
  WHERE it.organization_id = o.id AND it.code = 'PASSPORT'
);

INSERT IGNORE INTO identification_types (organization_id, code, name, description, is_active, is_default, validation_regex, validation_example, created_at, updated_at)
SELECT 
  o.id,
  'DRIVERS_LICENSE',
  'Driver''s License',
  'Driving license document',
  TRUE,
  FALSE,
  '^[A-Z0-9]{8,15}$',
  'DL12345678',
  UTC_TIMESTAMP(),
  UTC_TIMESTAMP()
FROM organizations o
WHERE NOT EXISTS (
  SELECT 1 FROM identification_types it 
  WHERE it.organization_id = o.id AND it.code = 'DRIVERS_LICENSE'
);

INSERT IGNORE INTO identification_types (organization_id, code, name, description, is_active, is_default, validation_regex, validation_example, created_at, updated_at)
SELECT 
  o.id,
  'EMPLOYEE_ID',
  'Employee ID',
  'Company-issued employee identification',
  TRUE,
  FALSE,
  '^[A-Z0-9-]{4,20}$',
  'EMP-12345',
  UTC_TIMESTAMP(),
  UTC_TIMESTAMP()
FROM organizations o
WHERE NOT EXISTS (
  SELECT 1 FROM identification_types it 
  WHERE it.organization_id = o.id AND it.code = 'EMPLOYEE_ID'
);

-- Add permissions for identification types, visits, and blacklist management
INSERT IGNORE INTO permissions(code, name, description, module, action, is_system, created_at) VALUES
('identification_types.view', 'Identification Types View', 'View identification types', 'identification_types', 'view', 1, UTC_TIMESTAMP()),
('identification_types.manage', 'Identification Types Manage', 'Create, update, and delete identification types', 'identification_types', 'manage', 1, UTC_TIMESTAMP()),
('visits.view', 'Visits View', 'View visit logs', 'visits', 'view', 1, UTC_TIMESTAMP()),
('visits.create', 'Visits Create', 'Create new visit records', 'visits', 'create', 1, UTC_TIMESTAMP()),
('visits.manage', 'Visits Manage', 'Update and manage visit records', 'visits', 'manage', 1, UTC_TIMESTAMP()),
('blacklist.view', 'Blacklist View', 'View blacklist entries', 'blacklist', 'view', 1, UTC_TIMESTAMP()),
('blacklist.manage', 'Blacklist Manage', 'Add, update, and remove blacklist entries', 'blacklist', 'manage', 1, UTC_TIMESTAMP());

-- Grant new permissions to organization_admin role
INSERT IGNORE INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'identification_types.view',
  'identification_types.manage',
  'visits.view',
  'visits.create',
  'visits.manage',
  'blacklist.view',
  'blacklist.manage'
)
WHERE r.code = 'organization_admin';
