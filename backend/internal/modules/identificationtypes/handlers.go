package identificationtypes

import (
	"errors"
	"net/http"

	"github.com/alumasinde/gopass/internal/platform/apperr"
	"github.com/alumasinde/gopass/internal/platform/audit"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/httpx"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	authz *rbac.Service
	audit *audit.Service
	svc   *Service
}

func NewHandler(db any, a *rbac.Service, au *audit.Service) *Handler {
	return &Handler{a, au, NewService(db)}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/identification-types", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/{id}", h.Get)
		r.Put("/{id}", h.Update)
		r.Delete("/{id}", h.Delete)
	})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "identification_types.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	org, _ := tenancy.ID(r.Context())
	items, err := h.svc.List(r.Context(), org)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, items)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "identification_types.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	org, _ := tenancy.ID(r.Context())
	id, err := httpx.ID(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	item, err := h.svc.Get(r.Context(), org, id)
	if err != nil {
		if err == ErrIdentificationTypeNotFound {
			httpx.Fail(w, apperr.NotFound)
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}
	httpx.OK(w, item)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "identification_types.manage") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	org, _ := tenancy.ID(r.Context())
	var in CreateInput
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	item := &IdentificationType{
		OrganizationID:    org,
		Code:              in.Code,
		Name:              in.Name,
		Description:       in.Description,
		IsActive:          in.IsActive,
		IsDefault:         in.IsDefault,
		ValidationRegex:   in.ValidationRegex,
		ValidationExample: in.ValidationExample,
	}
	if err := h.svc.Create(r.Context(), item); err != nil {
		switch {
		case errors.Is(err, ErrInvalidIdentificationType):
			httpx.Fail(w, apperr.InvalidRequest.With(err.Error()))
		case errors.Is(err, ErrDuplicateCode):
			httpx.Fail(w, apperr.Conflict.With(err.Error()))
		default:
			httpx.Fail(w, apperr.Database)
		}
		return
	}
	if h.audit != nil {
		claims, _ := auth.From(r.Context())
		h.audit.Record(r.Context(), audit.Entry{
			OrganizationID: org,
			ActorUserID:    claims.UserID,
			Action:         "identification_type.created",
			ResourceType:   "identification_type",
			ResourceID:     item.ID,
		})
	}
	httpx.Created(w, item)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "identification_types.manage") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	org, _ := tenancy.ID(r.Context())
	id, err := httpx.ID(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	var in UpdateInput
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	item, err := h.svc.Get(r.Context(), org, id)
	if err != nil {
		if err == ErrIdentificationTypeNotFound {
			httpx.Fail(w, apperr.NotFound)
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}
	item.Name = in.Name
	item.Description = in.Description
	if in.IsActive != nil {
		item.IsActive = *in.IsActive
	}
	if in.IsDefault != nil {
		item.IsDefault = *in.IsDefault
	}
	item.ValidationRegex = in.ValidationRegex
	item.ValidationExample = in.ValidationExample
	if err := h.svc.Update(r.Context(), item); err != nil {
		if errors.Is(err, ErrInvalidIdentificationType) {
			httpx.Fail(w, apperr.InvalidRequest.With(err.Error()))
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}
	if h.audit != nil {
		claims, _ := auth.From(r.Context())
		h.audit.Record(r.Context(), audit.Entry{
			OrganizationID: org,
			ActorUserID:    claims.UserID,
			Action:         "identification_type.updated",
			ResourceType:   "identification_type",
			ResourceID:     item.ID,
		})
	}
	httpx.OK(w, item)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "identification_types.manage") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	org, _ := tenancy.ID(r.Context())
	id, err := httpx.ID(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	if err := h.svc.Delete(r.Context(), org, id); err != nil {
		if err == ErrIdentificationTypeNotFound {
			httpx.Fail(w, apperr.NotFound)
		} else if errors.Is(err, ErrCannotDeleteDefault) {
			httpx.Fail(w, apperr.Conflict.With("cannot delete default identification type"))
		} else if errors.Is(err, ErrIdentificationTypeInUse) {
			httpx.Fail(w, apperr.Conflict.With("identification type is assigned to visitor records; deactivate it instead"))
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}
	if h.audit != nil {
		claims, _ := auth.From(r.Context())
		h.audit.Record(r.Context(), audit.Entry{
			OrganizationID: org,
			ActorUserID:    claims.UserID,
			Action:         "identification_type.deleted",
			ResourceType:   "identification_type",
			ResourceID:     id,
		})
	}
	httpx.OK(w, map[string]any{"message": "deleted"})
}
