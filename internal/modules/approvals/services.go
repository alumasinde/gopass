package approvals

import (
	"context"
	"database/sql"
	"errors"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/rbac"
)



type Service struct {
	db    *sql.DB
	authz *rbac.Service
}

func NewService(db *sql.DB, a *rbac.Service) *Service { return &Service{db, a} }
func (s *Service) List(c context.Context, o int64) ([]ApprovalRequest, error) {
	rows, e := s.db.QueryContext(c, `SELECT id,organization_id,gatepass_id,status,step_order,acted_by FROM approval_requests WHERE organization_id=? ORDER BY id DESC LIMIT 200`, o)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var x []ApprovalRequest
	for rows.Next() {
		var v ApprovalRequest
		if e := rows.Scan(&v.ID, &v.OrganizationID, &v.GatepassID, &v.Status, &v.StepOrder, &v.ActedBy); e != nil {
			return nil, e
		}
		x = append(x, v)
	}
	return x, rows.Err()
}
func (s *Service) Act(c context.Context, o, id int64, approve bool, note string) error {
	claims, ok := auth.From(c)
	if !ok {
		return ErrForbidden
	}
	tx, e := s.db.BeginTx(c, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var gpID, gateID, stepID, wfID int64
	var status, permission, scope string
	var order int
	e = tx.QueryRowContext(c, `SELECT ar.gatepass_id,ar.status,ar.step_order,ar.step_id,ar.workflow_id,gp.gate_id,COALESCE(aws.permission_code,''),COALESCE(aws.scope_type,'ORGANIZATION') FROM approval_requests ar JOIN gatepasses gp ON gp.id=ar.gatepass_id LEFT JOIN approval_workflow_steps aws ON aws.id=ar.step_id WHERE ar.organization_id=? AND ar.id=? FOR UPDATE`, o, id).Scan(&gpID, &status, &order, &stepID, &wfID, &gateID, &permission, &scope)
	if errors.Is(e, sql.ErrNoRows) {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	if status != "PENDING" {
		return ErrAlreadyActed
	}
	if permission == "" {
		permission = "approvals.approve"
	}
	if scope == "" {
		scope = "ORGANIZATION"
	}
	if e := s.authz.RequireWithScope(c, permission, scope, gateID); e != nil {
		return ErrForbidden
	}
	if approve {
		var nextStepID int64
		var nextOrder int
		var nextPermission, nextScope string
		err := tx.QueryRowContext(c, `SELECT id,step_order,permission_code,scope_type FROM approval_workflow_steps WHERE workflow_id=? AND is_active=1 AND step_order>? ORDER BY step_order LIMIT 1`, wfID, order).Scan(&nextStepID, &nextOrder, &nextPermission, &nextScope)
		if errors.Is(err, sql.ErrNoRows) {
			if _, e = tx.ExecContext(c, `UPDATE approval_requests SET status='APPROVED',acted_by=?,acted_at=UTC_TIMESTAMP(),decision_note=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, claims.UserID, note, o, id); e != nil {
				return e
			}
			if _, e = tx.ExecContext(c, `UPDATE gatepasses SET status='APPROVED',updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND status='PENDING_APPROVAL'`, o, gpID); e != nil {
				return e
			}
		} else if err != nil {
			return err
		} else {
			if _, e = tx.ExecContext(c, `UPDATE approval_requests SET status='APPROVED',acted_by=?,acted_at=UTC_TIMESTAMP(),decision_note=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, claims.UserID, note, o, id); e != nil {
				return e
			}
			if _, e = tx.ExecContext(c, `INSERT INTO approval_requests(organization_id,gatepass_id,status,step_order,workflow_id,step_id,created_at,updated_at) VALUES(?,?, 'PENDING',?,?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, o, gpID, nextOrder, wfID, nextStepID); e != nil {
				return e
			}
			_ = nextPermission
			_ = nextScope
		}
	} else {
		if _, e = tx.ExecContext(c, `UPDATE approval_requests SET status='REJECTED',acted_by=?,acted_at=UTC_TIMESTAMP(),decision_note=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, claims.UserID, note, o, id); e != nil {
			return e
		}
		if _, e = tx.ExecContext(c, `UPDATE gatepasses SET status='REJECTED',updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, o, gpID); e != nil {
			return e
		}
	}
	return tx.Commit()
}
