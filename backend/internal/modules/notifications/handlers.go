package notifications

import (
	"database/sql"
	"github.com/alumasinde/gopass/internal/app/database"
	"github.com/alumasinde/gopass/internal/platform/apperr"
	"github.com/alumasinde/gopass/internal/platform/httpx"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
	"github.com/go-chi/chi/v5"
	"net/http"
	"time"
)

type Handler struct {
	db    database.Handle
	authz *rbac.Service
}

func NewHandler(db any, a *rbac.Service) *Handler { return &Handler{db: database.Adapt(db), authz: a} }
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/notifications", func(r chi.Router) { r.Get("/", h.List); r.Post("/{id}/retry", h.Retry) })
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "notifications.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	org, err := tenancy.ID(r.Context())
	if err != nil {
		httpx.Fail(w, apperr.TenantMissing)
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT id,event_type,channel,recipient,subject,status,attempts,max_attempts,next_attempt_at,delivered_at,last_error,created_at FROM notifications WHERE organization_id=? ORDER BY id DESC LIMIT 100`, org)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var event, channel, recipient, subject, status string
		var attempts, max int
		var next, created timeValue
		var delivered sql.NullTime
		var last sql.NullString
		if err := rows.Scan(&id, &event, &channel, &recipient, &subject, &status, &attempts, &max, &next, &delivered, &last, &created); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		item := map[string]any{"id": id, "event_type": event, "channel": channel, "recipient": recipient, "subject": subject, "status": status, "attempts": attempts, "max_attempts": max, "next_attempt_at": next.Time, "created_at": created.Time}
		if delivered.Valid {
			item["delivered_at"] = delivered.Time
		}
		if last.Valid {
			item["last_error"] = last.String
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, out)
}

// timeValue keeps scan targets explicit without leaking payload data from the queue.
type timeValue struct{ Time time.Time }

func (t *timeValue) Scan(v any) error {
	var x sql.NullTime
	if err := x.Scan(v); err != nil {
		return err
	}
	if x.Valid {
		t.Time = x.Time
	}
	return nil
}
func (h *Handler) Retry(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "notifications.retry") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, err := httpx.ID(chi.URLParam(r, "id"))
	if err != nil || id < 1 {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	org, err := tenancy.ID(r.Context())
	if err != nil {
		httpx.Fail(w, apperr.TenantMissing)
		return
	}
	res, err := h.db.ExecContext(r.Context(), `UPDATE notifications SET status='RETRY',attempts=0,next_attempt_at=UTC_TIMESTAMP(),claimed_at=NULL,last_error=NULL,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND status='DEAD'`, org, id)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	httpx.OK(w, map[string]any{"id": id, "status": "RETRY"})
}
