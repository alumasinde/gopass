package checkouts

import (
	"database/sql"
	"fmt"
	"github.com/alumasinde/gopass/internal/platform/apperr"
	"github.com/alumasinde/gopass/internal/platform/audit"
	"github.com/alumasinde/gopass/internal/platform/httpx"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strings"
)

type Handler struct {
	db    *sql.DB
	authz *rbac.Service
	audit *audit.Service
}

func NewHandler(db *sql.DB, a *rbac.Service, au *audit.Service) *Handler { return &Handler{db, a, au} }
func (h *Handler) registerRoutes(r chi.Router) {
	r.Route("/check-outs", func(r chi.Router) { r.Get("/", h.List); r.Post("/", h.Create); r.Get("/{id}", h.Get) })
}

var fields = []string{"id", "gatepass_id", "gate_id", "checked_out_at"}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "checkouts.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	org, e := tenancy.ID(r.Context())
	if e != nil {
		httpx.Fail(w, apperr.TenantMissing)
		return
	}
	rows, e := h.db.QueryContext(r.Context(), `SELECT id, gatepass_id, gate_id, checked_out_at FROM check_outs WHERE organization_id=? ORDER BY id DESC LIMIT 100`, org)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(fields))
		ptr := make([]any, len(fields))
		for i := range vals {
			ptr[i] = &vals[i]
		}
		if rows.Scan(ptr...) != nil {
			continue
		}
		item := map[string]any{}
		for i, f := range fields {
			item[f] = vals[i]
		}
		out = append(out, item)
	}
	httpx.OK(w, out)
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "checkouts.perform") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	var in map[string]any
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	org, _ := tenancy.ID(r.Context())
	args := []any{org}
	for _, f := range []string{"gatepass_id", "gate_id"} {
		v, ok := in[f]
		if !ok || v == nil || strings.TrimSpace(fmt.Sprint(v)) == "" {
			httpx.Fail(w, apperr.InvalidRequest.With(f+" is required"))
			return
		}
		args = append(args, v)
	}
	res, e := h.db.ExecContext(r.Context(), `INSERT INTO check_outs(organization_id,gatepass_id, gate_id,created_at,updated_at) VALUES (?,?, ?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, args...)
	if e != nil {
		httpx.Fail(w, apperr.CreateFailed)
		return
	}
	id, _ := res.LastInsertId()
	h.audit.Record(r.Context(), audit.Entry{Action: "check_out.created", ResourceType: "check_out", ResourceID: id})
	httpx.Created(w, map[string]any{"id": id})
}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "checkouts.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	org, _ := tenancy.ID(r.Context())
	row := h.db.QueryRowContext(r.Context(), `SELECT id, gatepass_id, gate_id, checked_out_at FROM check_outs WHERE organization_id=? AND id=?`, org, id)
	vals := make([]any, len(fields))
	ptr := make([]any, len(fields))
	for i := range vals {
		ptr[i] = &vals[i]
	}
	if row.Scan(ptr...) != nil {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	item := map[string]any{}
	for i, f := range fields {
		item[f] = vals[i]
	}
	httpx.OK(w, item)
}
