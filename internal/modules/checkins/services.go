package checkins

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
func (s *Service) List(c context.Context, o int64) ([]CheckIn, error) {
	rows, e := s.db.QueryContext(c, `SELECT id,organization_id,gatepass_id,gate_id,checked_in_by,checked_in_at FROM check_ins WHERE organization_id=? ORDER BY id DESC LIMIT 200`, o)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var x []CheckIn
	for rows.Next() {
		var v CheckIn
		var t time.Time
		if e := rows.Scan(&v.ID, &v.OrganizationID, &v.GatepassID, &v.GateID, &v.CheckedInBy, &t); e != nil {
			return nil, e
		}
		v.CheckedInAt = t.UTC().Format(time.RFC3339)
		x = append(x, v)
	}
	return x, rows.Err()
}
func (s *Service) Create(c context.Context, o int64, token string, gate int64) (*CheckIn, error) {
	cl, ok := auth.From(c)
	if !ok {
		return nil, ErrInvalid
	}
	if e := s.authz.RequireWithScope(c, "checkins.perform", rbac.ScopeGate, gate); e != nil {
		return nil, e
	}
	tx, e := s.db.BeginTx(c, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var gpID, gpGate int64
	var status string
	var exp time.Time
	err := tx.QueryRowContext(c, `SELECT gp.id,gp.gate_id,gp.status,cr.expires_at FROM credentials cr JOIN gatepasses gp ON gp.id=cr.gatepass_id WHERE cr.organization_id=? AND cr.token=? AND cr.is_revoked=0 FOR UPDATE`, o, token).Scan(&gpID, &gpGate, &status, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if gpGate != gate || status != "ISSUED" || time.Now().UTC().After(exp) {
		return nil, ErrInvalid
	}
	var n int
	if err = tx.QueryRowContext(c, `SELECT COUNT(*) FROM check_ins WHERE organization_id=? AND gatepass_id=?`, o, gpID).Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, ErrAlready
	}
	res, err := tx.ExecContext(c, `INSERT INTO check_ins(organization_id,gatepass_id,gate_id,checked_in_by,checked_in_at) VALUES(?,?,?,?,UTC_TIMESTAMP())`, o, gpID, gate, cl.UserID)
	if err != nil {
		return nil, err
	}
	res2, err := tx.ExecContext(c, `UPDATE gatepasses SET status='CHECKED_IN',updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND status='ISSUED'`, o, gpID)
	if err != nil {
		return nil, err
	}
	if n, _ := res2.RowsAffected(); n != 1 {
		return nil, ErrInvalid
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	now := time.Now().UTC().Format(time.RFC3339)
	return &CheckIn{ID: id, OrganizationID: o, GatepassID: gpID, GateID: gate, CheckedInBy: &cl.UserID, CheckedInAt: now}, nil
}
