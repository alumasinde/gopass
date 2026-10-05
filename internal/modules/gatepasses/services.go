package gatepasses

import (
	"context"
	"database/sql"
	"errors"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"time"
)

const (
	StatusDraft           = "DRAFT"
	StatusPendingApproval = "PENDING_APPROVAL"
	StatusApproved        = "APPROVED"
	StatusRejected        = "REJECTED"
	StatusIssued          = "ISSUED"
	StatusCheckedIn       = "CHECKED_IN"
	StatusCheckedOut      = "CHECKED_OUT"
	StatusCancelled       = "CANCELLED"
	StatusRevoked         = "REVOKED"
)



type Service struct {
	db    *sql.DB
	authz *rbac.Service
}

func NewService(db *sql.DB, a *rbac.Service) *Service { return &Service{db, a} }
func (s *Service) List(c context.Context, o int64) ([]Gatepass, error) {
	rows, e := s.db.QueryContext(c, `SELECT id,organization_id,visitor_id,gate_id,pass_type_id,status,valid_from,valid_until FROM gatepasses WHERE organization_id=? ORDER BY id DESC LIMIT 200`, o)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var x []Gatepass
	for rows.Next() {
		var v Gatepass
		var vf, vu time.Time
		if e := rows.Scan(&v.ID, &v.OrganizationID, &v.VisitorID, &v.GateID, &v.PassTypeID, &v.Status, &vf, &vu); e != nil {
			return nil, e
		}
		v.ValidFrom = vf.UTC().Format(time.RFC3339)
		v.ValidUntil = vu.UTC().Format(time.RFC3339)
		x = append(x, v)
	}
	return x, rows.Err()
}
func (s *Service) Get(c context.Context, o, id int64) (*Gatepass, error) {
	var v Gatepass
	var vf, vu time.Time
	e := s.db.QueryRowContext(c, `SELECT id,organization_id,visitor_id,gate_id,pass_type_id,status,valid_from,valid_until FROM gatepasses WHERE organization_id=? AND id=?`, o, id).Scan(&v.ID, &v.OrganizationID, &v.VisitorID, &v.GateID, &v.PassTypeID, &v.Status, &vf, &vu)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if e == nil {
		v.ValidFrom = vf.UTC().Format(time.RFC3339)
		v.ValidUntil = vu.UTC().Format(time.RFC3339)
	}
	return &v, e
}
func (s *Service) Create(c context.Context, o, visitor, gate, passType int64, vf, vu time.Time) (*Gatepass, error) {
	if visitor < 1 || gate < 1 || passType < 1 || vf.IsZero() || vu.IsZero() || !vu.After(vf) {
		return nil, ErrInvalid
	}
	var n int
	if e := s.db.QueryRowContext(c, `SELECT COUNT(*) FROM visitors WHERE organization_id=? AND id=? AND is_blacklisted=0`, o, visitor).Scan(&n); e != nil {
		return nil, e
	}
	if n == 0 {
		return nil, ErrBlacklisted
	}
	if e := s.db.QueryRowContext(c, `SELECT COUNT(*) FROM gates WHERE organization_id=? AND id=? AND is_active=1`, o, gate).Scan(&n); e != nil || n == 0 {
		return nil, ErrInvalid
	}
	if e := s.db.QueryRowContext(c, `SELECT COUNT(*) FROM pass_types WHERE organization_id=? AND id=? AND is_active=1`, o, passType).Scan(&n); e != nil || n == 0 {
		return nil, ErrInvalid
	}
	res, e := s.db.ExecContext(c, `INSERT INTO gatepasses(organization_id,visitor_id,gate_id,pass_type_id,status,valid_from,valid_until,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, o, visitor, gate, passType, StatusDraft, vf, vu, time.Now().UTC(), time.Now().UTC())
	if e != nil {
		return nil, e
	}
	id, _ := res.LastInsertId()
	return s.Get(c, o, id)
}
func (s *Service) Submit(c context.Context, o, id int64) error {
	tx, e := s.db.BeginTx(c, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var status string
	var passType int64
	var gate int64
	var visitor int64
	var requires bool
	var vf, vu time.Time
	e = tx.QueryRowContext(c, `SELECT gp.status,gp.pass_type_id,gp.gate_id,gp.visitor_id,pt.requires_approval,gp.valid_from,gp.valid_until FROM gatepasses gp JOIN pass_types pt ON pt.id=gp.pass_type_id WHERE gp.organization_id=? AND gp.id=? FOR UPDATE`, o, id).Scan(&status, &passType, &gate, &visitor, &requires, &vf, &vu)
	if errors.Is(e, sql.ErrNoRows) {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	if status != StatusDraft {
		return ErrTransition
	}
	if !vu.After(time.Now().UTC()) {
		return ErrInvalid
	}
	var blacklisted bool
	if e = tx.QueryRowContext(c, `SELECT is_blacklisted FROM visitors WHERE organization_id=? AND id=?`, o, visitor).Scan(&blacklisted); e != nil {
		return e
	}
	if blacklisted {
		return ErrBlacklisted
	}
	if !requires {
		if _, e = tx.ExecContext(c, `UPDATE gatepasses SET status=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, StatusApproved, o, id); e != nil {
			return e
		}
		return tx.Commit()
	}
	var wfID, stepID int64
	var permission, scope string
	err := tx.QueryRowContext(c, `SELECT aw.id,aws.id,aws.permission_code,aws.scope_type FROM approval_workflows aw JOIN approval_workflow_steps aws ON aws.workflow_id=aw.id AND aws.step_order=1 AND aws.is_active=1 WHERE aw.organization_id=? AND (aw.pass_type_id=? OR aw.pass_type_id IS NULL) AND aw.is_active=1 ORDER BY CASE WHEN aw.pass_type_id=? THEN 0 ELSE 1 END,aw.id LIMIT 1`, o, passType, passType).Scan(&wfID, &stepID, &permission, &scope)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrApprovalRequired
	}
	if err != nil {
		return err
	}
	if _, e = tx.ExecContext(c, `UPDATE gatepasses SET status=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, StatusPendingApproval, o, id); e != nil {
		return e
	}
	_, e = tx.ExecContext(c, `INSERT INTO approval_requests(organization_id,gatepass_id,status,step_order,workflow_id,step_id,created_at,updated_at) VALUES(?,?,?,?,?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, o, id, "PENDING", 1, wfID, stepID)
	if e != nil {
		return e
	}
	_ = gate
	_ = permission
	_ = scope
	return tx.Commit()
}
func validTransition(from, to string) bool {
	switch from {
	case StatusDraft:
		return to == StatusCancelled
	case StatusApproved:
		return to == StatusIssued || to == StatusCancelled || to == StatusRevoked
	case StatusIssued:
		return to == StatusCheckedIn || to == StatusRevoked
	case StatusCheckedIn:
		return to == StatusCheckedOut
	default:
		return false
	}
}

func (s *Service) Transition(c context.Context, o, id int64, to string) error {
	v, e := s.Get(c, o, id)
	if e != nil {
		return e
	}
	if !validTransition(v.Status, to) {
		return ErrTransition
	}
	_, e = s.db.ExecContext(c, `UPDATE gatepasses SET status=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, to, o, id)
	return e
}
func (s *Service) RequireGateScope(c context.Context, permission string, gateID int64) error {
	return s.authz.RequireWithScope(c, permission, rbac.ScopeGate, gateID)
}
