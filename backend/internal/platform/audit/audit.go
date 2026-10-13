package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/alumasinde/gopass/internal/app/database"
	"log/slog"

	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
)

type Entry struct {
	OrganizationID       int64
	ActorUserID          int64
	Action, ResourceType string
	ResourceID           int64
	OldValues, NewValues any
}

type Service struct {
	DB  database.Handle
	Log *slog.Logger
}

func New(db any, l *slog.Logger) *Service { return &Service{DB: database.Adapt(db), Log: l} }

// RecordTx writes an audit event using the caller's transaction. A failed audit
// insert is returned to the caller so the business change can be rolled back.
func (s *Service) RecordTx(ctx context.Context, tx *sql.Tx, e Entry) error {
	if tx == nil {
		return errors.New("audit transaction is nil")
	}
	org, actor, old, nw, err := values(ctx, e)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(organization_id,actor_user_id,action,resource_type,resource_id,old_values,new_values,created_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP())`, org, actor, e.Action, e.ResourceType, nullableID(e.ResourceID), old, nw)
	return err
}

// Record is for events that are not part of a database state change. Workflow
// mutations must use RecordTx instead.
func (s *Service) Record(ctx context.Context, e Entry) {
	org, actor, old, nw, err := values(ctx, e)
	if err != nil {
		s.logError(ctx, "audit values encode failed", err, e.Action)
		return
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO audit_logs(organization_id,actor_user_id,action,resource_type,resource_id,old_values,new_values,created_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP())`, org, actor, e.Action, e.ResourceType, nullableID(e.ResourceID), old, nw); err != nil {
		s.logError(ctx, "audit write failed", err, e.Action)
	}
}

func values(ctx context.Context, e Entry) (int64, any, any, any, error) {
	org, _ := tenancy.ID(ctx)
	if org < 1 {
		org = e.OrganizationID
	}
	var actor any
	if claims, ok := auth.From(ctx); ok {
		actor = claims.UserID
	} else if e.ActorUserID > 0 {
		actor = e.ActorUserID
	}
	var old, nw any
	if e.OldValues != nil {
		b, err := json.Marshal(e.OldValues)
		if err != nil {
			return 0, nil, nil, nil, err
		}
		old = b
	}
	if e.NewValues != nil {
		b, err := json.Marshal(e.NewValues)
		if err != nil {
			return 0, nil, nil, nil, err
		}
		nw = b
	}
	return org, actor, old, nw, nil
}

func nullableID(id int64) any {
	if id < 1 {
		return nil
	}
	return id
}

func (s *Service) logError(ctx context.Context, msg string, err error, action string) {
	if s.Log != nil {
		s.Log.ErrorContext(ctx, msg, "error", err, "action", action)
	}
}
