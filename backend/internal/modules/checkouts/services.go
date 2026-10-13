package checkouts

import (
	"context"
	"database/sql"
	"errors"
	"github.com/alumasinde/gopass/internal/platform/audit"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"time"
)

type Service struct {
	db    *sql.DB
	authz *rbac.Service
	audit *audit.Service
}

func NewService(db *sql.DB, a *rbac.Service, audits ...*audit.Service) *Service {
	var au *audit.Service
	if len(audits) > 0 {
		au = audits[0]
	}
	return &Service{db: db, authz: a, audit: au}
}
func (s *Service) List(c context.Context, o int64) ([]CheckOut, error) {
	rows, e := s.db.QueryContext(c, `SELECT id,organization_id,gatepass_id,gate_id,checked_out_by,checked_out_at FROM check_outs WHERE organization_id=? ORDER BY id DESC LIMIT 200`, o)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var x []CheckOut
	for rows.Next() {
		var v CheckOut
		var t time.Time
		if e := rows.Scan(&v.ID, &v.OrganizationID, &v.GatepassID, &v.GateID, &v.CheckedOutBy, &t); e != nil {
			return nil, e
		}
		v.CheckedOutAt = t.UTC().Format(time.RFC3339)
		x = append(x, v)
	}
	return x, rows.Err()
}
func (s *Service) Create(c context.Context, o int64, gp, gate int64) (*CheckOut, error) {
	cl, ok := auth.From(c)
	if !ok {
		return nil, ErrInvalid
	}
	if e := s.authz.RequireWithScope(c, "checkouts.perform", rbac.ScopeGate, gate); e != nil {
		return nil, e
	}
	tx, e := s.db.BeginTx(c, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var actualGate int64
	var status string
	err := tx.QueryRowContext(c, `SELECT gate_id,status FROM gatepasses WHERE organization_id=? AND id=? FOR UPDATE`, o, gp).Scan(&actualGate, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if actualGate != gate || status != "CHECKED_IN" {
		return nil, ErrInvalid
	}
	var n int
	if err = tx.QueryRowContext(c, `SELECT COUNT(*) FROM check_outs WHERE organization_id=? AND gatepass_id=?`, o, gp).Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, ErrInvalid
	}
	res, err := tx.ExecContext(c, `INSERT INTO check_outs(organization_id,gatepass_id,gate_id,checked_out_by,checked_out_at) VALUES(?,?,?,?,UTC_TIMESTAMP())`, o, gp, gate, cl.UserID)
	if err != nil {
		return nil, err
	}
	res2, err := tx.ExecContext(c, `UPDATE gatepasses SET status='CHECKED_OUT',updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND status='CHECKED_IN'`, o, gp)
	if err != nil {
		return nil, err
	}
	if n, _ := res2.RowsAffected(); n != 1 {
		return nil, ErrInvalid
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if s.audit != nil {
		if err = s.audit.RecordTx(c, tx, audit.Entry{OrganizationID: o, Action: "check_out.created", ResourceType: "check_out", ResourceID: id, NewValues: map[string]any{"gatepass_id": gp, "gate_id": gate, "checked_out_by": cl.UserID, "status": "CHECKED_OUT"}}); err != nil {
			return nil, err
		}
		if err = s.audit.RecordTx(c, tx, audit.Entry{OrganizationID: o, Action: "gatepass.checked_out", ResourceType: "gatepass", ResourceID: gp, OldValues: map[string]any{"status": "CHECKED_IN"}, NewValues: map[string]any{"status": "CHECKED_OUT"}}); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return &CheckOut{ID: id, OrganizationID: o, GatepassID: gp, GateID: gate, CheckedOutBy: &cl.UserID, CheckedOutAt: now}, nil
}
