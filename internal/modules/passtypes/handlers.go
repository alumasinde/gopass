package passtypes

import (
	"database/sql"
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
	r.Route("/pass-types", func(r chi.Router) { r.Get("/", h.List); r.Post("/", h.Create) })
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "pass_types.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	org, _ := tenancy.ID(r.Context())
	rows, e := h.db.QueryContext(r.Context(), `SELECT id,name,code,requires_approval,validity_minutes,is_active FROM pass_types WHERE organization_id=? ORDER BY name`, org)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var n, c string
		var req, active bool
		var mins int
		if rows.Scan(&id, &n, &c, &req, &mins, &active) == nil {
			out = append(out, map[string]any{"id": id, "name": n, "code": c, "requires_approval": req, "validity_minutes": mins, "is_active": active})
		}
	}
	httpx.OK(w, out)
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "pass_types.create") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	var in struct {
		Name, Code       string
		RequiresApproval bool
		ValidityMinutes  int
	}
	if httpx.Decode(r, &in) != nil || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Code) == "" || in.ValidityMinutes < 1 {
		httpx.Fail(w, apperr.InvalidRequest.With("name, code and positive validity_minutes are required"))
		return
	}
	org, _ := tenancy.ID(r.Context())
	res, e := h.db.ExecContext(r.Context(), `INSERT INTO pass_types(organization_id,name,code,requires_approval,validity_minutes,is_active,created_at,updated_at) VALUES(?,?,?,?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, org, in.Name, in.Code, in.RequiresApproval, in.ValidityMinutes)
	if e != nil {
		httpx.Fail(w, apperr.Conflict.With("pass type could not be created"))
		return
	}
	id, _ := res.LastInsertId()
	h.audit.Record(r.Context(), audit.Entry{Action: "pass_type.created", ResourceType: "pass_type", ResourceID: id})
	httpx.Created(w, map[string]any{"id": id})
}
