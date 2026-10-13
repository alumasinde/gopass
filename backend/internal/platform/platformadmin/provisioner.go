package platformadmin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type dedicatedProvisioner interface {
	ProvisionDedicated(context.Context, int64) error
}

// Provisioner activates a tenant only after schema validation. Shared tenants
// use the shared tenant schema; dedicated tenants use the tenant-aware router and the
// embedded versioned migration runner.
type sharedProvisioner interface {
	ProvisionShared(context.Context, int64) error
}
type tenantBootstrapper interface {
	BootstrapTenantIdentity(context.Context, int64, string, string, string, string, string, string, string) error
}

type Provisioner struct {
	db        *sql.DB
	shared    *sql.DB
	log       *slog.Logger
	interval  time.Duration
	dedicated dedicatedProvisioner
}

func NewProvisioner(db *sql.DB, log *slog.Logger, dedicated ...dedicatedProvisioner) *Provisioner {
	p := &Provisioner{db: db, shared: db, log: log, interval: 5 * time.Second}
	if len(dedicated) > 0 {
		p.dedicated = dedicated[0]
		if source, ok := dedicated[0].(interface{ SharedDatabase() *sql.DB }); ok && source.SharedDatabase() != nil {
			p.shared = source.SharedDatabase()
		}
	}
	return p
}
func (p *Provisioner) Run(ctx context.Context) {
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := p.ProcessBatch(ctx); err != nil && p.log != nil {
				p.log.ErrorContext(ctx, "tenant provisioning batch failed", "error", err)
			}
		}
	}
}
func (p *Provisioner) ProcessBatch(ctx context.Context) error {
	if err := p.processBootstrapBatch(ctx); err != nil {
		return err
	}
	rows, err := p.db.QueryContext(ctx, `SELECT id,database_mode FROM organizations WHERE lifecycle_status='PROVISIONING' AND provisioning_status IN ('PENDING','PROVISIONING') AND (database_mode='SHARED' OR (database_mode='DEDICATED' AND database_config_ciphertext IS NOT NULL)) ORDER BY id LIMIT 25`)
	if err != nil {
		return err
	}
	type candidate struct {
		id   int64
		mode string
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.mode); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, c := range candidates {
		var e error
		if c.mode == "SHARED" {
			e = p.provisionShared(ctx, c.id)
		} else {
			e = p.provisionDedicated(ctx, c.id)
		}
		if e != nil && p.log != nil {
			p.log.ErrorContext(ctx, "tenant provisioning failed", "organization_id", c.id, "database_mode", c.mode, "error", e)
		}
	}
	return nil
}
func (p *Provisioner) processBootstrapBatch(ctx context.Context) error {
	rows, err := p.db.QueryContext(ctx, `SELECT o.id,o.name,o.slug,o.database_mode,b.first_name,b.last_name,b.email,b.password_hash FROM organizations o JOIN tenant_bootstrap_requests b ON b.organization_id=o.id WHERE o.lifecycle_status='PROVISIONING' AND o.provisioning_status='BOOTSTRAPPING' ORDER BY o.id LIMIT 25`)
	if err != nil {
		return err
	}
	type item struct {
		id                                         int64
		name, slug, mode, first, last, email, hash string
	}
	items := make([]item, 0)
	for rows.Next() {
		var x item
		if err := rows.Scan(&x.id, &x.name, &x.slug, &x.mode, &x.first, &x.last, &x.email, &x.hash); err != nil {
			rows.Close()
			return err
		}
		items = append(items, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, x := range items {
		bootstrapper, ok := p.dedicated.(tenantBootstrapper)
		var bootstrapErr error
		if !ok {
			bootstrapErr = errors.New("tenant database router unavailable for bootstrap")
		} else {
			bootstrapErr = bootstrapper.BootstrapTenantIdentity(ctx, x.id, x.name, x.slug, x.mode, x.first, x.last, x.email, x.hash)
		}
		tx, txErr := p.db.BeginTx(ctx, nil)
		if txErr != nil {
			return txErr
		}
		if bootstrapErr != nil {
			reason := bootstrapErr.Error()
			if len(reason) > 450 {
				reason = reason[:450]
			}
			_, txErr = tx.ExecContext(ctx, `UPDATE organizations SET is_active=0,lifecycle_status='FAILED',provisioning_status='FAILED',provisioning_error=?,updated_at=UTC_TIMESTAMP() WHERE id=? AND provisioning_status='BOOTSTRAPPING'`, reason, x.id)
			if txErr == nil {
				txErr = p.auditTx(ctx, tx, 0, "organization.bootstrap_failed", x.id, nil, map[string]any{"provisioning_status": "FAILED", "reason": reason})
			}
		} else {
			_, txErr = tx.ExecContext(ctx, `DELETE FROM tenant_bootstrap_requests WHERE organization_id=?`, x.id)
			if txErr == nil {
				_, txErr = tx.ExecContext(ctx, `UPDATE organizations SET provisioning_status='PENDING',provisioning_error=NULL,updated_at=UTC_TIMESTAMP() WHERE id=? AND provisioning_status='BOOTSTRAPPING'`, x.id)
			}
			if txErr == nil {
				txErr = p.auditTx(ctx, tx, 0, "organization.bootstrap_completed", x.id, map[string]any{"provisioning_status": "BOOTSTRAPPING"}, map[string]any{"provisioning_status": "PENDING"})
			}
		}
		if txErr != nil {
			_ = tx.Rollback()
			return txErr
		}
		if txErr = tx.Commit(); txErr != nil {
			return txErr
		}
		if bootstrapErr != nil && p.log != nil {
			p.log.ErrorContext(ctx, "tenant bootstrap failed", "organization_id", x.id, "error", bootstrapErr)
		}
	}
	return nil
}

func (p *Provisioner) provisionShared(ctx context.Context, id int64) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var mode, status, lifecycle string
	var active bool
	err = tx.QueryRowContext(ctx, `SELECT database_mode,provisioning_status,lifecycle_status,is_active FROM organizations WHERE id=? FOR UPDATE`, id).Scan(&mode, &status, &lifecycle, &active)
	if err != nil {
		return err
	}
	if mode != "SHARED" || lifecycle != "PROVISIONING" || (status != "PENDING" && status != "PROVISIONING") {
		return nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE organizations SET provisioning_status='PROVISIONING',lifecycle_status='PROVISIONING',is_active=0,updated_at=UTC_TIMESTAMP() WHERE id=?`, id); err != nil {
		return err
	}
	var count int
	if p.shared == nil {
		return p.fail(ctx, id, "shared tenant database is not configured")
	}
	if err = p.shared.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('organizations','users','gatepasses','notifications','audit_logs')`).Scan(&count); err != nil {
		return err
	}
	if count != 5 {
		reason := fmt.Sprintf("shared tenant schema incomplete: found %d of 5 required tables", count)
		if _, err = tx.ExecContext(ctx, `UPDATE organizations SET provisioning_status='FAILED',lifecycle_status='FAILED',provisioning_error=?,updated_at=UTC_TIMESTAMP() WHERE id=?`, reason, id); err != nil {
			return err
		}
		if err = p.auditTx(ctx, tx, 0, "organization.provisioning_failed", id, map[string]any{"provisioning_status": status}, map[string]any{"provisioning_status": "FAILED", "reason": reason}); err != nil {
			return err
		}
		return tx.Commit()
	}
	if sp, ok := p.dedicated.(sharedProvisioner); ok && p.shared != p.db {
		if err = sp.ProvisionShared(ctx, id); err != nil {
			reason := fmt.Sprintf("shared tenant identity provisioning failed: %v", err)
			if len(reason) > 450 {
				reason = reason[:450]
			}
			if _, updateErr := tx.ExecContext(ctx, `UPDATE organizations SET provisioning_status='FAILED',lifecycle_status='FAILED',provisioning_error=?,updated_at=UTC_TIMESTAMP() WHERE id=?`, reason, id); updateErr != nil {
				return updateErr
			}
			if auditErr := p.auditTx(ctx, tx, 0, "organization.provisioning_failed", id, nil, map[string]any{"provisioning_status": "FAILED", "reason": reason}); auditErr != nil {
				return auditErr
			}
			return tx.Commit()
		}
	} else if p.shared != p.db {
		if err = p.provisionSharedIdentity(ctx, id); err != nil {
			reason := fmt.Sprintf("shared tenant identity provisioning failed: %v", err)
			if len(reason) > 450 {
				reason = reason[:450]
			}
			if _, updateErr := tx.ExecContext(ctx, `UPDATE organizations SET provisioning_status='FAILED',lifecycle_status='FAILED',provisioning_error=?,updated_at=UTC_TIMESTAMP() WHERE id=?`, reason, id); updateErr != nil {
				return updateErr
			}
			if auditErr := p.auditTx(ctx, tx, 0, "organization.provisioning_failed", id, nil, map[string]any{"provisioning_status": "FAILED", "reason": reason}); auditErr != nil {
				return auditErr
			}
			return tx.Commit()
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE organizations SET provisioning_status='READY',lifecycle_status='ACTIVE',is_active=1,provisioning_error=NULL,provisioned_at=UTC_TIMESTAMP(),updated_at=UTC_TIMESTAMP() WHERE id=?`, id); err != nil {
		return err
	}
	if err = p.auditTx(ctx, tx, 0, "organization.provisioned", id, map[string]any{"provisioning_status": status, "lifecycle_status": lifecycle}, map[string]any{"provisioning_status": "READY", "lifecycle_status": "ACTIVE"}); err != nil {
		return err
	}
	return tx.Commit()
}
func (p *Provisioner) provisionSharedIdentity(ctx context.Context, id int64) error {
	var name, slug string
	if err := p.db.QueryRowContext(ctx, `SELECT name,slug FROM organizations WHERE id=? AND database_mode='SHARED'`, id).Scan(&name, &slug); err != nil {
		return err
	}
	tx, err := p.shared.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT slug FROM organizations WHERE id=? FOR UPDATE`, id).Scan(&existing)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `INSERT INTO organizations(id,name,slug,is_active,lifecycle_status,database_mode,provisioning_status,created_at,updated_at) VALUES(?,?,?,1,'ACTIVE','SHARED','READY',UTC_TIMESTAMP(),UTC_TIMESTAMP())`, id, name, slug)
	} else if existing != slug {
		return fmt.Errorf("organization ID %d already belongs to slug %q", id, existing)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE organizations SET name=?,slug=?,is_active=1,lifecycle_status='ACTIVE',database_mode='SHARED',provisioning_status='READY',updated_at=UTC_TIMESTAMP() WHERE id=?`, name, slug, id)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (p *Provisioner) provisionDedicated(ctx context.Context, id int64) error {
	if p.dedicated == nil {
		return p.fail(ctx, id, "dedicated database router is unavailable")
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var mode, status, lifecycle string
	if err = tx.QueryRowContext(ctx, `SELECT database_mode,provisioning_status,lifecycle_status FROM organizations WHERE id=? FOR UPDATE`, id).Scan(&mode, &status, &lifecycle); err != nil {
		return err
	}
	if mode != "DEDICATED" || lifecycle != "PROVISIONING" || (status != "PENDING" && status != "PROVISIONING") {
		return nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE organizations SET provisioning_status='PROVISIONING',is_active=0,updated_at=UTC_TIMESTAMP() WHERE id=?`, id); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if err = p.dedicated.ProvisionDedicated(ctx, id); err != nil {
		return p.fail(ctx, id, err.Error())
	}
	done, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer done.Rollback()
	var currentMode, currentLife, currentStatus string
	if err = done.QueryRowContext(ctx, `SELECT database_mode,lifecycle_status,provisioning_status FROM organizations WHERE id=? FOR UPDATE`, id).Scan(&currentMode, &currentLife, &currentStatus); err != nil {
		return err
	}
	if currentMode != "DEDICATED" || currentLife != "PROVISIONING" {
		return fmt.Errorf("tenant lifecycle changed during provisioning")
	}
	if _, err = done.ExecContext(ctx, `UPDATE organizations SET provisioning_status='READY',lifecycle_status='ACTIVE',is_active=1,provisioning_error=NULL,provisioned_at=UTC_TIMESTAMP(),updated_at=UTC_TIMESTAMP() WHERE id=?`, id); err != nil {
		return err
	}
	if err = p.auditTx(ctx, done, 0, "organization.provisioned", id, map[string]any{"database_mode": "DEDICATED", "provisioning_status": currentStatus}, map[string]any{"database_mode": "DEDICATED", "provisioning_status": "READY", "lifecycle_status": "ACTIVE"}); err != nil {
		return err
	}
	return done.Commit()
}
func (p *Provisioner) fail(ctx context.Context, id int64, reason string) error {
	if len(reason) > 450 {
		reason = reason[:450]
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var mode, status, life string
	if err = tx.QueryRowContext(ctx, `SELECT database_mode,provisioning_status,lifecycle_status FROM organizations WHERE id=? FOR UPDATE`, id).Scan(&mode, &status, &life); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE organizations SET is_active=0,lifecycle_status='FAILED',provisioning_status='FAILED',provisioning_error=?,updated_at=UTC_TIMESTAMP() WHERE id=?`, reason, id); err != nil {
		return err
	}
	if err = p.auditTx(ctx, tx, 0, "organization.provisioning_failed", id, map[string]any{"database_mode": mode, "provisioning_status": status, "lifecycle_status": life}, map[string]any{"provisioning_status": "FAILED", "reason": reason}); err != nil {
		return err
	}
	return tx.Commit()
}
func (p *Provisioner) auditTx(ctx context.Context, tx *sql.Tx, actor int64, action string, id int64, oldv, newv any) error {
	var oldJSON, newJSON any
	var err error
	if oldv != nil {
		oldJSON, err = json.Marshal(oldv)
		if err != nil {
			return err
		}
	}
	if newv != nil {
		newJSON, err = json.Marshal(newv)
		if err != nil {
			return err
		}
	}
	var actorID any
	if actor > 0 {
		actorID = actor
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO platform_audit_logs(actor_platform_user_id,action,resource_type,resource_id,old_values,new_values,created_at) VALUES(?,?,?,?,?,?,UTC_TIMESTAMP())`, actorID, action, "organization", id, oldJSON, newJSON)
	return err
}
