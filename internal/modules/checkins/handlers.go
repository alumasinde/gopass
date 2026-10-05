package checkins

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
	r.Route("/check-ins", func(r chi.Router) { r.Get("/", h.List); r.Post("/", h.Create); r.Get("/{id}", h.Get) })
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "checkins.view") != nil {
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
	if h.authz.Require(r.Context(), "checkins.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	o, _ := tenancy.ID(r.Context())
	v, e := h.service.List(r.Context(), o)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	for _, x := range v {
		if x.ID == id {
			httpx.OK(w, x)
			return
		}
	}
	httpx.Fail(w, apperr.NotFound)
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "checkins.perform") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	var in struct {
		CredentialToken string `json:"credential_token"`
		GateID          int64  `json:"gate_id"`
	}
	if httpx.Decode(r, &in) != nil || in.CredentialToken == "" || in.GateID < 1 {
		httpx.Fail(w, apperr.InvalidRequest.With("credential_token and gate_id are required"))
		return
	}
	o, _ := tenancy.ID(r.Context())
	v, e := h.service.Create(r.Context(), o, in.CredentialToken, in.GateID)
	switch {
	case errors.Is(e, ErrNotFound):
		httpx.Fail(w, apperr.NotFound)
	case errors.Is(e, ErrAlready):
		httpx.Fail(w, apperr.Conflict.With("visitor is already checked in"))
	case errors.Is(e, ErrInvalid):
		httpx.Fail(w, apperr.Conflict.With("credential or gatepass is not eligible for check-in"))
	case e != nil:
		httpx.Fail(w, apperr.Forbidden)
	default:
		h.audit.Record(r.Context(), audit.Entry{Action: "check_in.created", ResourceType: "check_in", ResourceID: v.ID})
		httpx.Created(w, v)
	}
}
