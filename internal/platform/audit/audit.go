package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
	"log/slog"
)

type Entry struct {
	OrganizationID       int64
	ActorUserID          int64
	Action, ResourceType string
	ResourceID           int64
	OldValues, NewValues any
}
type Service struct {
	DB  *sql.DB
	Log *slog.Logger
}

func New(db *sql.DB, l *slog.Logger) *Service { return &Service{db, l} }
func (s *Service) Record(ctx context.Context, e Entry) {
	org, _ := tenancy.ID(ctx)
	if org < 1 {
		org = e.OrganizationID
	}
	var uid any
	if c, ok := auth.From(ctx); ok {
		uid = c.UserID
	} else if e.ActorUserID > 0 {
		uid = e.ActorUserID
	}
	// Keep absent before/after snapshots as SQL NULL. Encoding a nil interface
	// directly with json.Marshal produces the JSON literal "null", which is
	// semantically different from an event having no old/new value.
	var old, nw any
	if e.OldValues != nil {
		encoded, err := json.Marshal(e.OldValues)
		if err != nil {
			s.Log.ErrorContext(ctx, "audit old values encode failed", "error", err, "action", e.Action)
			return
		}
		old = encoded
	}
	if e.NewValues != nil {
		encoded, err := json.Marshal(e.NewValues)
		if err != nil {
			s.Log.ErrorContext(ctx, "audit new values encode failed", "error", err, "action", e.Action)
			return
		}
		nw = encoded
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO audit_logs(organization_id,actor_user_id,action,resource_type,resource_id,old_values,new_values,created_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP())`, org, uid, e.Action, e.ResourceType, e.ResourceID, old, nw); err != nil {
		s.Log.ErrorContext(ctx, "audit write failed", "error", err, "action", e.Action)
	}
}
