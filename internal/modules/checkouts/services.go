package checkouts

import (
	"context"
	"database/sql"
	"errors"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"time"
)



type Service struct {
	db    *sql.DB
	authz *rbac.Service
}

func NewService(db *sql.DB, a *rbac.Service) *Service { return &Service{db, a} }
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
	cl, _ := auth.From(c)
	if e := s.authz.RequireWithScope(c, "checkouts.perform", rbac.ScopeGate, gate); e != nil {
		return nil, e
	}
	var actualGate int64
	var status string
	e := s.db.QueryRowContext(c, `SELECT gate_id,status FROM gatepasses WHERE organization_id=? AND id=?`, o, gp).Scan(&actualGate, &status)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	if actualGate != gate || status != "CHECKED_IN" {
		return nil, ErrInvalid
	}
	var n int
	if e := s.db.QueryRowContext(c, `SELECT COUNT(*) FROM check_outs WHERE organization_id=? AND gatepass_id=?`, o, gp).Scan(&n); e != nil {
		return nil, e
	}
	if n > 0 {
		return nil, ErrInvalid
	}
	tx, e := s.db.BeginTx(c, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	res, e := tx.ExecContext(c, `INSERT INTO check_outs(organization_id,gatepass_id,gate_id,checked_out_by,checked_out_at) VALUES(?,?,?,?,UTC_TIMESTAMP())`, o, gp, gate, cl.UserID)
	if e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(c, `UPDATE gatepasses SET status='CHECKED_OUT',updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND status='CHECKED_IN'`, o, gp); e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	id, _ := res.LastInsertId()
	return &CheckOut{ID: id, OrganizationID: o, GatepassID: gp, GateID: gate, CheckedOutBy: &cl.UserID, CheckedOutAt: time.Now().UTC().Format(time.RFC3339)}, nil
}
