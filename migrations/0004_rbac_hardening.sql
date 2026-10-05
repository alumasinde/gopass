-- 0004_rbac_hardening: pass_types.update / pass_types.delete permissions (re-runnable).
INSERT IGNORE INTO permissions(code,name,description,module,action,is_system,created_at) VALUES
('pass_types.update','Pass Types Update','Update pass types','pass_types','update',1,UTC_TIMESTAMP()),
('pass_types.delete','Pass Types Delete','Archive pass types','pass_types','delete',1,UTC_TIMESTAMP());
