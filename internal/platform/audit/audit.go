package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/alumasinde/passnow/internal/platform/auth"
	"github.com/alumasinde/passnow/internal/platform/tenancy"
	"log/slog"
)

type Entry struct {
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
	var uid any
	if c, ok := auth.From(ctx); ok {
		uid = c.UserID
	}
	old, _ := json.Marshal(e.OldValues)
	nw, _ := json.Marshal(e.NewValues)
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO audit_logs(organization_id,actor_user_id,action,resource_type,resource_id,old_values,new_values,created_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP())`, org, uid, e.Action, e.ResourceType, e.ResourceID, old, nw); err != nil {
		s.Log.ErrorContext(ctx, "audit write failed", "error", err, "action", e.Action)
	}
}
