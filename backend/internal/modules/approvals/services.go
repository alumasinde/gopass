package approvals

import (
	"context"
	"database/sql"
	"errors"

	"github.com/alumasinde/gopass/internal/modules/notifications"
	"github.com/alumasinde/gopass/internal/platform/audit"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/rbac"
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

func (s *Service) Get(c context.Context, o, id int64) (*ApprovalRequest, error) {
	var v ApprovalRequest
	query := `SELECT id,organization_id,gatepass_id,status,step_order,acted_by FROM approval_requests WHERE organization_id=? AND id=?`
	err := s.db.QueryRowContext(c, query, o, id).Scan(&v.ID, &v.OrganizationID, &v.GatepassID, &v.Status, &v.StepOrder, &v.ActedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}
func (s *Service) Act(c context.Context, o, id int64, approve bool, note string) error {
	claims, ok := auth.From(c)
	if !ok {
		return ErrForbidden
	}
	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var gpID, gateID, stepID, wfID, createdBy int64
	var status, permission, scope string
	var order int
	err = tx.QueryRowContext(c, `SELECT ar.gatepass_id,ar.status,ar.step_order,COALESCE(ar.step_id,0),COALESCE(ar.workflow_id,0),gp.gate_id,COALESCE(gp.created_by,0),COALESCE(aws.permission_code,''),COALESCE(aws.scope_type,'ORGANIZATION') FROM approval_requests ar JOIN gatepasses gp ON gp.id=ar.gatepass_id AND gp.organization_id=ar.organization_id LEFT JOIN approval_workflow_steps aws ON aws.id=ar.step_id AND aws.workflow_id=ar.workflow_id WHERE ar.organization_id=? AND ar.id=? FOR UPDATE`, o, id).Scan(&gpID, &status, &order, &stepID, &wfID, &gateID, &createdBy, &permission, &scope)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "PENDING" {
		return ErrAlreadyActed
	}

	// Serialize all decisions for this pass. This prevents two separate requests
	// from advancing the same approval workflow concurrently.
	var gpStatus string
	err = tx.QueryRowContext(c, `SELECT status FROM gatepasses WHERE organization_id=? AND id=? FOR UPDATE`, o, gpID).Scan(&gpStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if gpStatus != "PENDING_APPROVAL" {
		return ErrAlreadyActed
	}
	if approve && createdBy == claims.UserID {
		return ErrForbidden
	}
	if permission == "" {
		permission = "approvals.approve"
	}
	if scope == "" {
		scope = "ORGANIZATION"
	}
	if err := s.authz.RequireWithScope(c, permission, scope, gateID); err != nil {
		return ErrForbidden
	}

	if approve {
		var nextStepID int64
		var nextOrder int
		err = tx.QueryRowContext(c, `SELECT aws.id,aws.step_order FROM approval_workflow_steps aws JOIN approval_workflows aw ON aw.id=aws.workflow_id AND aw.organization_id=? WHERE aws.workflow_id=? AND aws.is_active=1 AND aws.step_order>? ORDER BY aws.step_order LIMIT 1`, o, wfID, order).Scan(&nextStepID, &nextOrder)
		if errors.Is(err, sql.ErrNoRows) {
			res, e := tx.ExecContext(c, `UPDATE approval_requests SET status='APPROVED',acted_by=?,acted_at=UTC_TIMESTAMP(),decision_note=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND status='PENDING'`, claims.UserID, note, o, id)
			if e != nil {
				return e
			}
			if n, e := res.RowsAffected(); e != nil || n != 1 {
				if e != nil {
					return e
				}
				return ErrAlreadyActed
			}
			res, e = tx.ExecContext(c, `UPDATE gatepasses SET status='APPROVED',updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND status='PENDING_APPROVAL'`, o, gpID)
			if e != nil {
				return e
			}
			if n, e := res.RowsAffected(); e != nil || n != 1 {
				if e != nil {
					return e
				}
				return ErrAlreadyActed
			}
			if s.audit != nil {
				if e = s.audit.RecordTx(c, tx, audit.Entry{OrganizationID: o, Action: "gatepass.approved", ResourceType: "gatepass", ResourceID: gpID, OldValues: map[string]any{"status": "PENDING_APPROVAL"}, NewValues: map[string]any{"status": "APPROVED"}}); e != nil {
					return e
				}
			}
			if e = notifications.EnqueueVisitorTx(c, tx, o, gpID, "approval.approved", "Your gatepass request was approved", map[string]any{"gatepass_id": gpID, "status": "APPROVED"}); e != nil {
				return e
			}
		} else if err != nil {
			return err
		} else {
			res, e := tx.ExecContext(c, `UPDATE approval_requests SET status='APPROVED',acted_by=?,acted_at=UTC_TIMESTAMP(),decision_note=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND status='PENDING'`, claims.UserID, note, o, id)
			if e != nil {
				return e
			}
			if n, e := res.RowsAffected(); e != nil || n != 1 {
				if e != nil {
					return e
				}
				return ErrAlreadyActed
			}
			if _, e = tx.ExecContext(c, `INSERT INTO approval_requests(organization_id,gatepass_id,status,step_order,workflow_id,step_id,created_at,updated_at) VALUES(?,?, 'PENDING',?,?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, o, gpID, nextOrder, wfID, nextStepID); e != nil {
				return e
			}
		}
		if s.audit != nil {
			if err = s.audit.RecordTx(c, tx, audit.Entry{OrganizationID: o, Action: "approval.approved", ResourceType: "approval_request", ResourceID: id, OldValues: map[string]any{"status": "PENDING"}, NewValues: map[string]any{"status": "APPROVED", "acted_by": claims.UserID, "decision_note": note}}); err != nil {
				return err
			}
		}
	} else {
		res, e := tx.ExecContext(c, `UPDATE approval_requests SET status='REJECTED',acted_by=?,acted_at=UTC_TIMESTAMP(),decision_note=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND status='PENDING'`, claims.UserID, note, o, id)
		if e != nil {
			return e
		}
		if n, e := res.RowsAffected(); e != nil || n != 1 {
			if e != nil {
				return e
			}
			return ErrAlreadyActed
		}
		res, e = tx.ExecContext(c, `UPDATE gatepasses SET status='REJECTED',updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND status='PENDING_APPROVAL'`, o, gpID)
		if e != nil {
			return e
		}
		if n, e := res.RowsAffected(); e != nil || n != 1 {
			if e != nil {
				return e
			}
			return ErrAlreadyActed
		}
		if s.audit != nil {
			if err = s.audit.RecordTx(c, tx, audit.Entry{OrganizationID: o, Action: "gatepass.rejected", ResourceType: "gatepass", ResourceID: gpID, OldValues: map[string]any{"status": "PENDING_APPROVAL"}, NewValues: map[string]any{"status": "REJECTED"}}); err != nil {
				return err
			}
			if err = s.audit.RecordTx(c, tx, audit.Entry{OrganizationID: o, Action: "approval.rejected", ResourceType: "approval_request", ResourceID: id, OldValues: map[string]any{"status": "PENDING"}, NewValues: map[string]any{"status": "REJECTED", "acted_by": claims.UserID, "decision_note": note}}); err != nil {
				return err
			}
		}
		if err = notifications.EnqueueVisitorTx(c, tx, o, gpID, "approval.rejected", "Your gatepass request was rejected", map[string]any{"gatepass_id": gpID, "status": "REJECTED"}); err != nil {
			return err
		}
	}
	return tx.Commit()
}
