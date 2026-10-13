package platformadmin

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// Provisioner activates shared-database tenants only after the required schema
// exists. Dedicated tenants stay BLOCKED until a dedicated data-plane router and
// tenant-specific migration runner are available; it never falsely marks them ready.
type Provisioner struct {
	db       *sql.DB
	log      *slog.Logger
	interval time.Duration
}

func NewProvisioner(db *sql.DB, log *slog.Logger) *Provisioner {
	return &Provisioner{db: db, log: log, interval: 5 * time.Second}
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
	rows, err := p.db.QueryContext(ctx, `SELECT id FROM organizations WHERE database_mode='SHARED' AND lifecycle_status='PROVISIONING' AND provisioning_status IN ('PENDING','PROVISIONING') ORDER BY id LIMIT 25`)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range ids {
		if err := p.provisionShared(ctx, id); err != nil && p.log != nil {
			p.log.ErrorContext(ctx, "tenant provisioning failed", "organization_id", id, "error", err)
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
	var missing int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('users','gatepasses','notifications','audit_logs')`).Scan(&missing)
	if err != nil {
		return err
	}
	if missing != 4 {
		reason := fmt.Sprintf("shared schema incomplete: found %d of 4 required tables", missing)
		if _, err = tx.ExecContext(ctx, `UPDATE organizations SET provisioning_status='FAILED',lifecycle_status='FAILED',provisioning_error=?,updated_at=UTC_TIMESTAMP() WHERE id=?`, reason, id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO platform_audit_logs(actor_platform_user_id,action,resource_type,resource_id,old_values,new_values,created_at) VALUES(NULL,'organization.provisioning_failed','organization',?,JSON_OBJECT('provisioning_status',?),JSON_OBJECT('provisioning_status','FAILED','reason',?),UTC_TIMESTAMP())`, id, status, reason); err != nil {
			return err
		}
		return tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, `UPDATE organizations SET provisioning_status='READY',lifecycle_status='ACTIVE',is_active=1,provisioning_error=NULL,provisioned_at=UTC_TIMESTAMP(),updated_at=UTC_TIMESTAMP() WHERE id=?`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO platform_audit_logs(actor_platform_user_id,action,resource_type,resource_id,old_values,new_values,created_at) VALUES(NULL,'organization.provisioned','organization',?,JSON_OBJECT('provisioning_status',?,'lifecycle_status',?),JSON_OBJECT('provisioning_status','READY','lifecycle_status','ACTIVE'),UTC_TIMESTAMP())`, id, status, lifecycle); err != nil {
		return err
	}
	return tx.Commit()
}
