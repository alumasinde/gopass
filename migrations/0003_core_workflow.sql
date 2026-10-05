USE gopass;

CREATE TABLE IF NOT EXISTS approval_workflows (
 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
 organization_id BIGINT UNSIGNED NOT NULL,
 pass_type_id BIGINT UNSIGNED NULL,
 name VARCHAR(150) NOT NULL,
 is_active BOOLEAN NOT NULL DEFAULT TRUE,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL,
 UNIQUE KEY uq_workflow_org_name(organization_id,name),
 FOREIGN KEY(organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
 FOREIGN KEY(pass_type_id) REFERENCES pass_types(id) ON DELETE SET NULL
);
CREATE TABLE IF NOT EXISTS approval_workflow_steps (
 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
 workflow_id BIGINT UNSIGNED NOT NULL,
 step_order INT UNSIGNED NOT NULL,
 name VARCHAR(150) NOT NULL,
 permission_code VARCHAR(120) NOT NULL,
 scope_type VARCHAR(30) NOT NULL DEFAULT 'ORGANIZATION',
 is_active BOOLEAN NOT NULL DEFAULT TRUE,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL,
 UNIQUE KEY uq_workflow_step(workflow_id,step_order),
 FOREIGN KEY(workflow_id) REFERENCES approval_workflows(id) ON DELETE CASCADE
);
ALTER TABLE approval_requests ADD COLUMN workflow_id BIGINT UNSIGNED NULL;
ALTER TABLE approval_requests ADD COLUMN step_id BIGINT UNSIGNED NULL;
ALTER TABLE approval_requests ADD COLUMN decision_note VARCHAR(500) NULL;
ALTER TABLE approval_requests ADD COLUMN assigned_to BIGINT UNSIGNED NULL;
ALTER TABLE approval_requests ADD KEY idx_approval_org_gatepass(organization_id,gatepass_id,status,step_order);
ALTER TABLE gatepasses ADD KEY idx_gatepasses_org_gate(organization_id,gate_id,status);
ALTER TABLE credentials ADD KEY idx_credentials_org_gatepass(organization_id,gatepass_id,is_revoked);
ALTER TABLE check_ins ADD KEY idx_checkins_org_gatepass(organization_id,gatepass_id,checked_in_at);
ALTER TABLE check_outs ADD KEY idx_checkouts_org_gatepass(organization_id,gatepass_id,checked_out_at);

INSERT IGNORE INTO permissions(code,name,description,module,action,is_system,created_at) VALUES
('organization.update','Organization Update','Update organization settings','organization','update',1,UTC_TIMESTAMP()),
('users.update','Users Update','Update users','users','update',1,UTC_TIMESTAMP()),
('users.disable','Users Disable','Enable or disable users','users','disable',1,UTC_TIMESTAMP()),
('roles.update','Roles Update','Update roles','roles','update',1,UTC_TIMESTAMP()),
('roles.delete','Roles Delete','Delete roles','roles','delete',1,UTC_TIMESTAMP()),
('sites.update','Sites Update','Update sites','sites','update',1,UTC_TIMESTAMP()),
('sites.delete','Sites Delete','Delete sites','sites','delete',1,UTC_TIMESTAMP()),
('gates.update','Gates Update','Update gates','gates','update',1,UTC_TIMESTAMP()),
('gates.delete','Gates Delete','Delete gates','gates','delete',1,UTC_TIMESTAMP()),
('visitors.update','Visitors Update','Update visitors','visitors','update',1,UTC_TIMESTAMP()),
('visitors.archive','Visitors Archive','Archive visitors','visitors','archive',1,UTC_TIMESTAMP()),
('visitors.blacklist','Visitors Blacklist','Blacklist or unblacklist visitors','visitors','blacklist',1,UTC_TIMESTAMP()),
('gatepasses.update','Gatepasses Update','Update draft gatepasses','gatepasses','update',1,UTC_TIMESTAMP()),
('gatepasses.submit','Gatepasses Submit','Submit gatepasses','gatepasses','submit',1,UTC_TIMESTAMP()),
('gatepasses.cancel','Gatepasses Cancel','Cancel gatepasses','gatepasses','cancel',1,UTC_TIMESTAMP()),
('gatepasses.revoke','Gatepasses Revoke','Revoke gatepasses','gatepasses','revoke',1,UTC_TIMESTAMP()),
('approvals.reject','Approvals Reject','Reject approval requests','approvals','reject',1,UTC_TIMESTAMP()),
('credentials.verify','Credentials Verify','Verify QR credentials','credentials','verify',1,UTC_TIMESTAMP()),
('credentials.revoke','Credentials Revoke','Revoke credentials','credentials','revoke',1,UTC_TIMESTAMP()),
('checkins.override','Checkins Override','Override check-in rules','checkins','override',1,UTC_TIMESTAMP()),
('checkouts.override','Checkouts Override','Override check-out rules','checkouts','override',1,UTC_TIMESTAMP());
