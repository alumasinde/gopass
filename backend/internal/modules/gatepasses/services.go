package gatepasses

import (
	"context"
	"database/sql"
	"errors"
	"github.com/alumasinde/gopass/internal/app/database"
	"strings"
	"time"

	"github.com/alumasinde/gopass/internal/modules/notifications"
	"github.com/alumasinde/gopass/internal/platform/audit"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/rbac"
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
	db    database.Handle
	authz *rbac.Service
	audit *audit.Service
}

func NewService(db any, a *rbac.Service, audits ...*audit.Service) *Service {
	var au *audit.Service
	if len(audits) > 0 {
		au = audits[0]
	}
	return &Service{db: database.Adapt(db), authz: a, audit: au}
}

func (s *Service) List(c context.Context, o int64) ([]Gatepass, error) {
	all, gateIDs, err := s.accessibleGates(c, o)
	if err != nil {
		return nil, err
	}
	if !all && len(gateIDs) == 0 {
		return []Gatepass{}, nil
	}

	query := `SELECT id,organization_id,visitor_id,gate_id,pass_type_id,status,valid_from,valid_until FROM gatepasses WHERE organization_id=?`
	args := []any{o}
	if !all {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(gateIDs)), ",")
		query += ` AND gate_id IN (` + ph + `)`
		for _, g := range gateIDs {
			args = append(args, g)
		}
	}
	query += ` ORDER BY id DESC LIMIT 200`

	rows, e := s.db.QueryContext(c, query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	x := []Gatepass{}
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

func (s *Service) accessibleGates(c context.Context, orgID int64) (all bool, ids []int64, err error) {
	return s.authz.AccessibleGates(c, "gatepasses.view")
}
func (s *Service) getUserAccessibleGates(c context.Context, orgID int64) ([]int64, error) {
	// This would need to extract user ID from context and query their gate-scoped roles
	// For now, return empty to allow org-wide access (can be enhanced later)
	return []int64{}, nil
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
	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Lock and validate all referenced records inside the same transaction.
	var blacklisted bool
	if err = tx.QueryRowContext(c, `SELECT is_blacklisted FROM visitors WHERE organization_id=? AND id=? FOR UPDATE`, o, visitor).Scan(&blacklisted); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalid
	} else if err != nil {
		return nil, err
	}
	if blacklisted {
		return nil, ErrBlacklisted
	}
	var active bool
	if err = tx.QueryRowContext(c, `SELECT is_active FROM gates WHERE organization_id=? AND id=? FOR UPDATE`, o, gate).Scan(&active); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalid
	} else if err != nil {
		return nil, err
	}
	if !active {
		return nil, ErrInvalid
	}
	if err = tx.QueryRowContext(c, `SELECT is_active FROM pass_types WHERE organization_id=? AND id=? FOR UPDATE`, o, passType).Scan(&active); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalid
	} else if err != nil {
		return nil, err
	}
	if !active {
		return nil, ErrInvalid
	}

	var creator any
	if claims, ok := auth.From(c); ok && claims.UserID > 0 {
		creator = claims.UserID
	}
	now := time.Now().UTC()
	res, err := tx.ExecContext(c, `INSERT INTO gatepasses(organization_id,visitor_id,gate_id,pass_type_id,created_by,status,valid_from,valid_until,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, o, visitor, gate, passType, creator, StatusDraft, vf.UTC(), vu.UTC(), now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if s.audit != nil {
		if err = s.audit.RecordTx(c, tx, audit.Entry{OrganizationID: o, Action: "gatepass.created", ResourceType: "gatepass", ResourceID: id, NewValues: map[string]any{"status": StatusDraft, "visitor_id": visitor, "gate_id": gate, "pass_type_id": passType, "valid_from": vf.UTC(), "valid_until": vu.UTC()}}); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(c, o, id)
}
func (s *Service) Submit(c context.Context, o, id int64) (string, error) {
	var gateID int64
	err := s.db.QueryRowContext(c, `SELECT gate_id FROM gatepasses WHERE organization_id=? AND id=?`, o, id).Scan(&gateID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if err := s.RequireGateScope(c, "gatepasses.submit", gateID); err != nil {
		return "", err
	}

	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var (
		status          string
		passType, visit int64
		requires        bool
		validUntil      time.Time
	)
	err = tx.QueryRowContext(c, `SELECT gp.status,gp.pass_type_id,gp.visitor_id,pt.requires_approval,gp.valid_until FROM gatepasses gp JOIN pass_types pt ON pt.id=gp.pass_type_id AND pt.organization_id=gp.organization_id WHERE gp.organization_id=? AND gp.id=? FOR UPDATE`, o, id).
		Scan(&status, &passType, &visit, &requires, &validUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if status != StatusDraft {
		return "", ErrTransition
	}
	if !validUntil.After(time.Now().UTC()) {
		return "", ErrInvalid
	}

	var blacklisted bool
	if err = tx.QueryRowContext(c, `SELECT is_blacklisted FROM visitors WHERE organization_id=? AND id=?`, o, visit).Scan(&blacklisted); err != nil {
		return "", err
	}
	if blacklisted {
		return "", ErrBlacklisted
	}

	next := StatusApproved
	if requires {
		var wfID, stepID int64
		var stepOrder int
		err = tx.QueryRowContext(c, `SELECT aw.id,aws.id,aws.step_order FROM approval_workflows aw JOIN approval_workflow_steps aws ON aws.workflow_id=aw.id AND aws.is_active=1 WHERE aw.organization_id=? AND (aw.pass_type_id=? OR aw.pass_type_id IS NULL) AND aw.is_active=1 ORDER BY CASE WHEN aw.pass_type_id=? THEN 0 ELSE 1 END,aws.step_order,aw.id LIMIT 1`, o, passType, passType).
			Scan(&wfID, &stepID, &stepOrder)
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrApprovalRequired
		}
		if err != nil {
			return "", err
		}
		if _, err = tx.ExecContext(c, `INSERT INTO approval_requests(organization_id,gatepass_id,status,step_order,workflow_id,step_id,created_at,updated_at) VALUES(?,?,'PENDING',?,?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, o, id, stepOrder, wfID, stepID); err != nil {
			return "", err
		}
		next = StatusPendingApproval
	}

	res, err := tx.ExecContext(c, `UPDATE gatepasses SET status=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND status=?`, next, o, id, StatusDraft)
	if err != nil {
		return "", err
	}
	if n, err := res.RowsAffected(); err != nil {
		return "", err
	} else if n != 1 {
		return "", ErrTransition
	}
	if s.audit != nil {
		if err = s.audit.RecordTx(c, tx, audit.Entry{OrganizationID: o, Action: "gatepass.submitted", ResourceType: "gatepass", ResourceID: id, OldValues: map[string]any{"status": StatusDraft}, NewValues: map[string]any{"status": next}}); err != nil {
			return "", err
		}
	}
	event := "gatepass.submitted"
	subject := "Your gatepass request was submitted"
	if next == StatusPendingApproval {
		event = "approval.requested"
		subject = "Your gatepass is awaiting approval"
	}
	if err = notifications.EnqueueVisitorTx(c, tx, o, id, event, subject, map[string]any{"gatepass_id": id, "status": next}); err != nil {
		return "", err
	}
	if next == StatusPendingApproval {
		if err = notifications.EnqueueApproversTx(c, tx, o, id, gateID); err != nil {
			return "", err
		}
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return next, nil
}
func validTransition(from, to string) bool {
	switch from {
	case StatusDraft:
		return to == StatusCancelled

	case StatusPendingApproval:
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
	var gateID int64
	if err := s.db.QueryRowContext(c, `SELECT gate_id FROM gatepasses WHERE organization_id=? AND id=?`, o, id).Scan(&gateID); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	permission := "gatepasses.cancel"
	if to == StatusRevoked {
		permission = "gatepasses.revoke"
	}
	if err := s.RequireGateScope(c, permission, gateID); err != nil {
		return err
	}
	tx, e := s.db.BeginTx(c, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var from string
	err := tx.QueryRowContext(c, `SELECT status FROM gatepasses WHERE organization_id=? AND id=? FOR UPDATE`, o, id).Scan(&from)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !validTransition(from, to) {
		return ErrTransition
	}
	res, err := tx.ExecContext(c, `UPDATE gatepasses SET status=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND status=?`, to, o, id, from)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		if err != nil {
			return err
		}
		return ErrTransition
	}
	// If revoking, also revoke associated credentials
	if to == StatusRevoked {
		_, err = tx.ExecContext(c, `UPDATE credentials SET is_revoked=1,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND gatepass_id=?`, o, id)
		if err != nil {
			return err
		}
	}
	if s.audit != nil {
		action := "gatepass.cancelled"
		if to == StatusRevoked {
			action = "gatepass.revoked"
		}
		if err = s.audit.RecordTx(c, tx, audit.Entry{OrganizationID: o, Action: action, ResourceType: "gatepass", ResourceID: id, OldValues: map[string]any{"status": from}, NewValues: map[string]any{"status": to}}); err != nil {
			return err
		}
	}
	if to == StatusIssued {
		if err = notifications.EnqueueVisitorTx(c, tx, o, id, "gatepass.issued", "Your gatepass has been issued", map[string]any{"gatepass_id": id, "status": to}); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Service) RequireGateScope(c context.Context, permission string, gateID int64) error {
	return s.authz.RequireWithScope(c, permission, rbac.ScopeGate, gateID)
}
