-- 0014: Visitor company, identification, and risk scoring support.
-- Idempotent because the migration runner may be used after a partially completed deployment.

SET @s := (SELECT IF(COUNT(*)=0, 'ALTER TABLE visitors ADD COLUMN company_name VARCHAR(180) NULL AFTER last_name', 'SELECT 1') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='visitors' AND column_name='company_name');
PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;

SET @s := (SELECT IF(COUNT(*)=0, 'ALTER TABLE visitors ADD KEY idx_visitors_org_identity(organization_id,id_type_id,id_number)', 'SELECT 1') FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='visitors' AND index_name='idx_visitors_org_identity');
PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;

-- Keep actor references nullable so deleting a user does not prevent user removal.
SET @s := (SELECT IF(COUNT(*)>0, 'ALTER TABLE blacklist MODIFY COLUMN added_by BIGINT UNSIGNED NULL', 'SELECT 1') FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='blacklist' AND column_name='added_by');
PREPARE st FROM @s; EXECUTE st; DEALLOCATE PREPARE st;

-- Expand the organization's identification-type catalogue. Regexes are deliberately
-- permissive: organizations may tune validation to their document policies.
INSERT IGNORE INTO identification_types
(organization_id,code,name,description,is_active,is_default,validation_regex,validation_example,created_at,updated_at)
SELECT o.id, seed.code, seed.name, seed.description, 1, 0, seed.validation_regex, seed.validation_example, UTC_TIMESTAMP(), UTC_TIMESTAMP()
FROM organizations o
CROSS JOIN (
  SELECT 'ALIEN_ID' code, 'Alien / Foreign National ID' name, 'Kenya foreign national identification document' description, '^[A-Z0-9-]{4,30}$' validation_regex, 'A12345678' validation_example
  UNION ALL SELECT 'REFUGEE_ID','Refugee Identification','Refugee identification document','^[A-Z0-9-]{4,30}$','R12345678'
  UNION ALL SELECT 'DRIVING_LICENCE','Driving Licence','Driving licence issued by an authorized authority','^[A-Z0-9-]{4,30}$','DL-12345678'
  UNION ALL SELECT 'WORK_PERMIT','Work Permit','Work or employment permit','^[A-Z0-9-]{4,30}$','WP-12345678'
  UNION ALL SELECT 'STUDENT_ID','Student ID','School, college or university identification','^[A-Z0-9-]{3,30}$','STU-12345'
  UNION ALL SELECT 'EMPLOYEE_ID','Employee ID','Employer-issued identification','^[A-Z0-9-]{3,30}$','EMP-12345'
  UNION ALL SELECT 'MILITARY_ID','Military / Service ID','Military or authorized service identification','^[A-Z0-9-]{4,30}$','MIL-12345'
  UNION ALL SELECT 'DIPLOMATIC_ID','Diplomatic ID','Diplomatic identification document','^[A-Z0-9-]{4,30}$','DIP-12345'
  UNION ALL SELECT 'OTHER','Other Identification','Other organization-approved identification document','^[A-Z0-9-]{2,50}$','ID-12345'
) seed
WHERE NOT EXISTS (
  SELECT 1 FROM identification_types existing
  WHERE existing.organization_id=o.id AND existing.code=seed.code
);

INSERT IGNORE INTO permissions(code,name,description,module,action,is_system,created_at) VALUES
('identification_types.view','Identification Types View','View identification types','identification_types','view',1,UTC_TIMESTAMP()),
('identification_types.manage','Identification Types Manage','Create, update and delete identification types','identification_types','manage',1,UTC_TIMESTAMP()),
('blacklist.view','Blacklist View','View blacklist records and risk results','blacklist','view',1,UTC_TIMESTAMP()),
('blacklist.manage','Blacklist Manage','Manage blacklist records','blacklist','manage',1,UTC_TIMESTAMP());

INSERT IGNORE INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r JOIN permissions p ON p.code IN
('identification_types.view','identification_types.manage','blacklist.view','blacklist.manage')
WHERE r.code='organization_admin';
