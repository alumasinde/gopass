-- 0016_visits_permissions: make Visits permissions available on every tenant schema.
-- Re-runnable: permission definitions and role grants are inserted idempotently.
INSERT IGNORE INTO permissions(code, name, description, module, action, is_system, created_at) VALUES
('visits.view', 'Visits View', 'View visit records and visit logs', 'visits', 'view', 1, UTC_TIMESTAMP()),
('visits.create', 'Visits Create', 'Create new visit records', 'visits', 'create', 1, UTC_TIMESTAMP()),
('visits.manage', 'Visits Manage', 'Update, check out, and delete visit records', 'visits', 'manage', 1, UTC_TIMESTAMP());

-- Organization administrators receive the full Visits permission set by default.
-- Other roles retain their existing explicit assignments and least-privilege behavior.
INSERT IGNORE INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN ('visits.view', 'visits.create', 'visits.manage')
WHERE r.code = 'organization_admin';
