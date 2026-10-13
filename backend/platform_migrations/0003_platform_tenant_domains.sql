CREATE TABLE IF NOT EXISTS platform_tenant_domains (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
 organization_id BIGINT UNSIGNED NOT NULL,
 domain VARCHAR(253) NOT NULL UNIQUE,
 verification_token VARCHAR(96) NOT NULL,
 is_verified BOOLEAN NOT NULL DEFAULT FALSE,
 is_active BOOLEAN NOT NULL DEFAULT FALSE,
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL,
 KEY idx_platform_tenant_domains_org(organization_id,is_active,is_verified),
 CONSTRAINT fk_platform_tenant_domains_org FOREIGN KEY(organization_id) REFERENCES organizations(id) ON DELETE CASCADE
) ENGINE=InnoDB;
