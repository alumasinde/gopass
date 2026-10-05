package credentials

import (
	"database/sql"
	"fmt"
	"github.com/alumasinde/passnow/internal/platform/audit"
	"github.com/alumasinde/passnow/internal/platform/httpx"
	"github.com/alumasinde/passnow/internal/platform/rbac"
	"github.com/alumasinde/passnow/internal/platform/tenancy"
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
	r.Route("/credentials", func(r chi.Router) { r.Get("/", h.List); r.Post("/", h.Create); r.Get("/{id}", h.Get) })
}

var fields = []string{"id", "gatepass_id", "token", "expires_at", "is_revoked"}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "credentials.view") != nil {
		httpx.Error(w, 403, "forbidden", "permission denied")
		return
	}
	org, e := tenancy.ID(r.Context())
	if e != nil {
		httpx.Error(w, 500, "tenant_missing", e.Error())
		return
	}
	rows, e := h.db.QueryContext(r.Context(), `SELECT id, gatepass_id, token, expires_at, is_revoked FROM credentials WHERE organization_id=? ORDER BY id DESC LIMIT 100`, org)
	if e != nil {
		httpx.Error(w, 500, "database_error", "database error")
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
	if h.authz.Require(r.Context(), "credentials.issue") != nil {
		httpx.Error(w, 403, "forbidden", "permission denied")
		return
	}
	var in map[string]any
	if httpx.Decode(r, &in) != nil {
		httpx.Error(w, 400, "invalid_request", "invalid JSON")
		return
	}
	org, _ := tenancy.ID(r.Context())
	args := []any{org}
	for _, f := range []string{"gatepass_id", "token", "expires_at"} {
		v, ok := in[f]
		if !ok || v == nil || strings.TrimSpace(fmt.Sprint(v)) == "" {
			httpx.Error(w, 400, "invalid_request", f+" is required")
			return
		}
		args = append(args, v)
	}
	res, e := h.db.ExecContext(r.Context(), `INSERT INTO credentials(organization_id,gatepass_id, token, expires_at,created_at,updated_at) VALUES (?,?, ?, ?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, args...)
	if e != nil {
		httpx.Error(w, 409, "conflict", "resource could not be created")
		return
	}
	id, _ := res.LastInsertId()
	h.audit.Record(r.Context(), audit.Entry{Action: "credential.created", ResourceType: "credential", ResourceID: id})
	httpx.Created(w, map[string]any{"id": id})
}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "credentials.view") != nil {
		httpx.Error(w, 403, "forbidden", "permission denied")
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Error(w, 400, "invalid_id", "invalid id")
		return
	}
	org, _ := tenancy.ID(r.Context())
	row := h.db.QueryRowContext(r.Context(), `SELECT id, gatepass_id, token, expires_at, is_revoked FROM credentials WHERE organization_id=? AND id=?`, org, id)
	vals := make([]any, len(fields))
	ptr := make([]any, len(fields))
	for i := range vals {
		ptr[i] = &vals[i]
	}
	if row.Scan(ptr...) != nil {
		httpx.Error(w, 404, "not_found", "resource not found")
		return
	}
	item := map[string]any{}
	for i, f := range fields {
		item[f] = vals[i]
	}
	httpx.OK(w, item)
}
