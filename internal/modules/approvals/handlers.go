package approvals

import (
	"database/sql"
	"errors"
	"github.com/alumasinde/gopass/internal/platform/apperr"
	"github.com/alumasinde/gopass/internal/platform/audit"
	"github.com/alumasinde/gopass/internal/platform/httpx"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
	"github.com/go-chi/chi/v5"
	"net/http"
)

type Handler struct {
	authz   *rbac.Service
	audit   *audit.Service
	service *Service
}

func NewHandler(db *sql.DB, a *rbac.Service, au *audit.Service) *Handler {
	return &Handler{a, au, NewService(db, a)}
}
func (h *Handler) registerRoutes(r chi.Router) {
	r.Route("/approvals", func(r chi.Router) {
		r.Get("/", h.List)
		r.Get("/{id}", h.Get)
		r.Post("/{id}/approve", h.Approve)
		r.Post("/{id}/reject", h.Reject)
	})
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "approvals.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	o, _ := tenancy.ID(r.Context())
	v, e := h.service.List(r.Context(), o)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, v)
}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "approvals.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	o, _ := tenancy.ID(r.Context())
	items, e := h.service.List(r.Context(), o)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	for _, v := range items {
		if v.ID == id {
			httpx.OK(w, v)
			return
		}
	}
	httpx.Fail(w, apperr.NotFound)
}
func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) { h.act(w, r, true) }
func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "approvals.reject") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	h.act(w, r, false)
}
func (h *Handler) act(w http.ResponseWriter, r *http.Request, approve bool) {
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	var in struct {
		Note string `json:"note"`
	}
	if r.Body != nil {
		_ = httpx.Decode(r, &in)
	}
	o, _ := tenancy.ID(r.Context())
	e = h.service.Act(r.Context(), o, id, approve, in.Note)
	switch {
	case errors.Is(e, ErrNotFound):
		httpx.Fail(w, apperr.NotFound)
	case errors.Is(e, ErrAlreadyActed):
		httpx.Fail(w, apperr.Conflict.With("approval request already acted on"))
	case errors.Is(e, ErrForbidden):
		httpx.Fail(w, apperr.Forbidden)
	case e != nil:
		httpx.Fail(w, apperr.Database)
	default:
		action := "approval.rejected"
		if approve {
			action = "approval.approved"
		}
		h.audit.Record(r.Context(), audit.Entry{Action: action, ResourceType: "approval_request", ResourceID: id})
		httpx.OK(w, map[string]any{"id": id, "status": "recorded"})
	}
}
