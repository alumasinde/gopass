package routes

import (
	"github.com/alumasinde/gopass/internal/app/http/middleware"
	"github.com/alumasinde/gopass/internal/modules/users"
	"github.com/alumasinde/gopass/internal/platform/audit"
	"github.com/alumasinde/gopass/internal/platform/httpx"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"github.com/go-chi/chi/v5"
	"net/http"
)

type Dependencies struct {
	Auth                                                                                                                    *users.AuthService
	Authz                                                                                                                   *rbac.Service
	Audit                                                                                                                   *audit.Service
	Users, Organizations, Roles, Sites, Gates, Visitors, Gatepasses, Approvals, Credentials, Checkins, Checkouts, PassTypes interface{ RegisterRoutes(chi.Router) }
}

func RegisterAPI(r chi.Router, d Dependencies) {
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, 200, map[string]any{"status": "ok", "service": "gopass"})
	})
	r.Route("/api/v1", func(r chi.Router) {
		d.Auth.Register(r)
		r.Group(func(r chi.Router) {
			r.Use(middleware.Bearer(d.Auth.TokenService()))
			d.Users.RegisterRoutes(r)
			d.Organizations.RegisterRoutes(r)
			d.Roles.RegisterRoutes(r)
			d.Sites.RegisterRoutes(r)
			d.Gates.RegisterRoutes(r)
			d.Visitors.RegisterRoutes(r)
			d.Gatepasses.RegisterRoutes(r)
			d.Approvals.RegisterRoutes(r)
			d.Credentials.RegisterRoutes(r)
			d.Checkins.RegisterRoutes(r)
			d.Checkouts.RegisterRoutes(r)
			d.PassTypes.RegisterRoutes(r)
		})
	})
}
