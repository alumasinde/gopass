package users

import (
	"database/sql"
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
	r.Route("/users", func(r chi.Router) { r.Get("/", h.List); r.Post("/", h.Create); r.Get("/{id}", h.Get) })
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "users.view") != nil {
		httpx.Error(w, 403, "forbidden", "permission denied")
		return
	}
	org, _ := tenancy.ID(r.Context())
	rows, e := h.db.QueryContext(r.Context(), `SELECT id,first_name,last_name,email,is_active FROM users WHERE organization_id=? ORDER BY id DESC LIMIT 100`, org)
	if e != nil {
		httpx.Error(w, 500, "database_error", "database error")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var fn, ln, email string
		var active bool
		if rows.Scan(&id, &fn, &ln, &email, &active) == nil {
			out = append(out, map[string]any{"id": id, "first_name": fn, "last_name": ln, "email": email, "is_active": active})
		}
	}
	httpx.OK(w, out)
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "users.create") != nil {
		httpx.Error(w, 403, "forbidden", "permission denied")
		return
	}
	var in struct{ FirstName, LastName, Email, Password string }
	if httpx.Decode(r, &in) != nil || in.FirstName == "" || in.LastName == "" || in.Email == "" || len(in.Password) < 12 {
		httpx.Error(w, 400, "invalid_request", "first_name, last_name, email and password of at least 12 characters are required")
		return
	}
	org, _ := tenancy.ID(r.Context())
	h, e := auth.Hash(in.Password)
	if e != nil {
		httpx.Error(w, 500, "internal_error", "password hashing failed")
		return
	}
	res, e := h.db.ExecContext(r.Context(), `INSERT INTO users(organization_id,first_name,last_name,email,password_hash,is_active,created_at,updated_at) VALUES(?,?,?,?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, org, in.FirstName, in.LastName, strings.ToLower(strings.TrimSpace(in.Email)), h)
	if e != nil {
		httpx.Error(w, 409, "conflict", "user could not be created")
		return
	}
	id, _ := res.LastInsertId()
	h.audit.Record(r.Context(), audit.Entry{Action: "user.created", ResourceType: "user", ResourceID: id})
	httpx.Created(w, map[string]any{"id": id})
}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "users.view") != nil {
		httpx.Error(w, 403, "forbidden", "permission denied")
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Error(w, 400, "invalid_id", "invalid id")
		return
	}
	org, _ := tenancy.ID(r.Context())
	var fn, ln, email string
	var active bool
	if e = h.db.QueryRowContext(r.Context(), `SELECT first_name,last_name,email,is_active FROM users WHERE organization_id=? AND id=?`, org, id).Scan(&fn, &ln, &email, &active); e != nil {
		httpx.Error(w, 404, "not_found", "user not found")
		return
	}
	httpx.OK(w, map[string]any{"id": id, "first_name": fn, "last_name": ln, "email": email, "is_active": active})
}
