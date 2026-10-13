package platformadmin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

type dedicatedProvisioner interface {
	ProvisionDedicated(context.Context, int64) error
}

// Provisioner activates a tenant only after schema validation. Shared tenants
// use the central schema; dedicated tenants use the tenant-aware router and the
// embedded versioned migration runner.
type Provisioner struct {
	db        *sql.DB
	log       *slog.Logger
	interval  time.Duration
	dedicated dedicatedProvisioner
}

func NewProvisioner(db *sql.DB, log *slog.Logger, dedicated ...dedicatedProvisioner) *Provisioner {
	p := &Provisioner{db: db, log: log, interval: 5 * time.Second}
	if len(dedicated) > 0 {
		p.dedicated = dedicated[0]
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
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('users','gatepasses','notifications','audit_logs')`).Scan(&count); err != nil {
		return err
	}
	if count != 4 {
		reason := fmt.Sprintf("shared schema incomplete: found %d of 4 required tables", count)
		if _, err = tx.ExecContext(ctx, `UPDATE organizations SET provisioning_status='FAILED',lifecycle_status='FAILED',provisioning_error=?,updated_at=UTC_TIMESTAMP() WHERE id=?`, reason, id); err != nil {
			return err
		}
		if err = p.auditTx(ctx, tx, 0, "organization.provisioning_failed", id, map[string]any{"provisioning_status": status}, map[string]any{"provisioning_status": "FAILED", "reason": reason}); err != nil {
			return err
		}
		return tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, `UPDATE organizations SET provisioning_status='READY',lifecycle_status='ACTIVE',is_active=1,provisioning_error=NULL,provisioned_at=UTC_TIMESTAMP(),updated_at=UTC_TIMESTAMP() WHERE id=?`, id); err != nil {
		return err
	}
	if err = p.auditTx(ctx, tx, 0, "organization.provisioned", id, map[string]any{"provisioning_status": status, "lifecycle_status": lifecycle}, map[string]any{"provisioning_status": "READY", "lifecycle_status": "ACTIVE"}); err != nil {
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
