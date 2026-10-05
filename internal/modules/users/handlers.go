package users

import (
	"database/sql"
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
	db      *sql.DB
	authz   *rbac.Service
	audit   *audit.Service
	Service *Service
	Auth    *auth.Service
}

func NewHandler(db *sql.DB, a *rbac.Service, au *audit.Service) *Handler {
	repo := NewRepository(db)
	return &Handler{db: db, authz: a, audit: au, Service: NewService(repo, nil)}
}

func NewAuthService(db *sql.DB, tokens *auth.Service) *Handler {
	repo := NewRepository(db)
	return &Handler{db: db, Service: NewService(repo, tokens), Auth: tokens}
}

func (h *Handler) registerRoutes(r chi.Router) {
	r.Route("/users", func(r chi.Router) { r.Get("/", h.List); r.Post("/", h.Create); r.Get("/{id}", h.Get) })
	r.Post("/auth/login", h.Login)
	r.Post("/auth/refresh", h.Refresh)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email            string `json:"email"`
		Password         string `json:"password"`
		OrganizationID   int64  `json:"organization_id"`
		OrganizationSlug string `json:"organization_slug"`
	}
	if httpx.Decode(r, &in) != nil || in.Email == "" || in.Password == "" {
		httpx.Fail(w, apperr.InvalidRequest.With("email and password are required"))
		return
	}

	org, e := tenancy.ID(r.Context())
	if e != nil {
		org = in.OrganizationID
		if org < 1 && in.OrganizationSlug != "" {
			e = h.db.QueryRowContext(r.Context(), `SELECT id FROM organizations WHERE slug=? AND is_active=1`, in.OrganizationSlug).Scan(&org)
		}
		if org < 1 || e != nil {
			httpx.Fail(w, apperr.InvalidRequest.With("organization_id or organization_slug is required"))
			return
		}
	}

	user, accessToken, refreshToken, e := h.Service.Authenticate(r.Context(), org, in.Email, in.Password)
	if e != nil {
		if e == ErrUserNotFound || e == ErrInvalidPassword {
			httpx.Fail(w, apperr.Unauthorized.With("invalid credentials"))
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}

	if h.audit != nil {
		h.audit.Record(r.Context(), audit.Entry{Action: "user.login", ResourceType: "user", ResourceID: user.ID})
	}
	httpx.OK(w, map[string]any{
		"user":          map[string]any{"id": user.ID, "first_name": user.FirstName, "last_name": user.LastName, "email": user.Email},
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	if httpx.Decode(r, &in) != nil || in.RefreshToken == "" {
		httpx.Fail(w, apperr.InvalidRequest.With("refresh_token is required"))
		return
	}

	newToken, e := h.Service.RefreshToken(r.Context(), in.RefreshToken)
	if e != nil {
		httpx.Fail(w, apperr.Unauthorized.With("invalid refresh token"))
		return
	}

	httpx.OK(w, map[string]any{"access_token": newToken})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.From(r.Context())
	if !ok {
		httpx.Fail(w, apperr.Unauthorized)
		return
	}
	org, err := tenancy.ID(r.Context())
	if err != nil || org != claims.OrgID {
		httpx.Fail(w, apperr.Unauthorized)
		return
	}
	user, err := h.Service.GetUser(r.Context(), org, claims.UserID)
	if err != nil || user == nil {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	httpx.OK(w, map[string]any{"id": user.ID, "organization_id": org, "first_name": user.FirstName, "last_name": user.LastName, "email": user.Email, "is_active": user.IsActive})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "users.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	org, _ := tenancy.ID(r.Context())
	users, e := h.Service.ListUsers(r.Context(), org, 100)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	out := make([]map[string]any, len(users))
	for i, u := range users {
		out[i] = map[string]any{"id": u.ID, "first_name": u.FirstName, "last_name": u.LastName, "email": u.Email, "is_active": u.IsActive}
	}
	httpx.OK(w, out)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "users.create") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	var in struct{ FirstName, LastName, Email, Password string }
	if httpx.Decode(r, &in) != nil || in.FirstName == "" || in.LastName == "" || in.Email == "" || len(in.Password) < 12 {
		httpx.Fail(w, apperr.InvalidRequest.With("first_name, last_name, email and password of at least 12 characters are required"))
		return
	}
	org, _ := tenancy.ID(r.Context())
	user, e := h.Service.CreateUser(r.Context(), org, in.FirstName, in.LastName, in.Email, in.Password)
	if e != nil {
		if e == ErrUserAlreadyExists {
			httpx.Fail(w, apperr.Conflict.With("user already exists"))
		} else if e == ErrInvalidEmail {
			httpx.Fail(w, apperr.InvalidRequest.With("invalid email"))
		} else if e == ErrInvalidPassword {
			httpx.Fail(w, apperr.InvalidRequest.With("password must be at least 12 characters"))
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "user.created", ResourceType: "user", ResourceID: user.ID})
	httpx.Created(w, map[string]any{"id": user.ID})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "users.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	org, _ := tenancy.ID(r.Context())
	user, e := h.Service.GetUser(r.Context(), org, id)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if user == nil {
		httpx.Fail(w, apperr.NotFound.With("user not found"))
		return
	}
	httpx.OK(w, map[string]any{"id": user.ID, "first_name": user.FirstName, "last_name": user.LastName, "email": user.Email, "is_active": user.IsActive})
}
