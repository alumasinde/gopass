package gatepasses

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
	"time"
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
	r.Route("/gatepasses", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/{id}", h.Get)
		r.Post("/{id}/submit", h.Submit)
		r.Post("/{id}/cancel", h.Cancel)
		r.Post("/{id}/revoke", h.Revoke)
	})
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "gatepasses.view") != nil {
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
	if h.authz.Require(r.Context(), "gatepasses.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	o, _ := tenancy.ID(r.Context())
	v, e := h.service.Get(r.Context(), o, id)
	if errors.Is(e, ErrNotFound) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, v)
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "gatepasses.create") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	var in struct {
		VisitorID, GateID, PassTypeID int64 `json:"visitor_id"`
		ValidFrom, ValidUntil         string
	}
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	vf, e := time.Parse(time.RFC3339, in.ValidFrom)
	if e != nil {
		httpx.Fail(w, apperr.InvalidRequest.With("valid_from must be RFC3339"))
		return
	}
	vu, e := time.Parse(time.RFC3339, in.ValidUntil)
	if e != nil {
		httpx.Fail(w, apperr.InvalidRequest.With("valid_until must be RFC3339"))
		return
	}
	o, _ := tenancy.ID(r.Context())
	v, e := h.service.Create(r.Context(), o, in.VisitorID, in.GateID, in.PassTypeID, vf, vu)
	if errors.Is(e, ErrBlacklisted) {
		httpx.Fail(w, apperr.Conflict.With("visitor is blacklisted"))
		return
	}
	if errors.Is(e, ErrInvalid) {
		httpx.Fail(w, apperr.InvalidRequest)
		return
	}
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "gatepass.created", ResourceType: "gatepass", ResourceID: v.ID})
	httpx.Created(w, v)
}
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "gatepasses.submit") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	o, _ := tenancy.ID(r.Context())
	e = h.service.Submit(r.Context(), o, id)
	switch {
	case errors.Is(e, ErrNotFound):
		httpx.Fail(w, apperr.NotFound)
	case errors.Is(e, ErrTransition):
		httpx.Fail(w, apperr.Conflict.With("gatepass cannot be submitted from its current state"))
	case errors.Is(e, ErrBlacklisted):
		httpx.Fail(w, apperr.Conflict.With("visitor is blacklisted"))
	case errors.Is(e, ErrApprovalRequired):
		httpx.Fail(w, apperr.Conflict.With("approval workflow is not configured"))
	case errors.Is(e, ErrInvalid):
		httpx.Fail(w, apperr.InvalidRequest)
	case e != nil:
		httpx.Fail(w, apperr.Database)
	default:
		h.audit.Record(r.Context(), audit.Entry{Action: "gatepass.submitted", ResourceType: "gatepass", ResourceID: id})
		httpx.OK(w, map[string]any{"id": id, "status": "PENDING_APPROVAL_OR_APPROVED"})
	}
}
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, "gatepasses.cancel", StatusCancelled, "gatepass.cancelled")
}
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, "gatepasses.revoke", StatusRevoked, "gatepass.revoked")
}
func (h *Handler) transition(w http.ResponseWriter, r *http.Request, p, to, action string) {
	if h.authz.Require(r.Context(), p) != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	o, _ := tenancy.ID(r.Context())
	e = h.service.Transition(r.Context(), o, id, to)
	if errors.Is(e, ErrNotFound) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if errors.Is(e, ErrTransition) {
		httpx.Fail(w, apperr.Conflict.With("invalid gatepass state transition"))
		return
	}
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: action, ResourceType: "gatepass", ResourceID: id})
	httpx.OK(w, map[string]any{"id": id, "status": to})
}
