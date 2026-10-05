package routes

import (
	"github.com/alumasinde/passnow/internal/app/http/middleware"
	"github.com/alumasinde/passnow/internal/modules/approvals"
	"github.com/alumasinde/passnow/internal/modules/checkins"
	"github.com/alumasinde/passnow/internal/modules/checkouts"
	"github.com/alumasinde/passnow/internal/modules/credentials"
	"github.com/alumasinde/passnow/internal/modules/gatepasses"
	"github.com/alumasinde/passnow/internal/modules/gates"
	"github.com/alumasinde/passnow/internal/modules/organizations"
	"github.com/alumasinde/passnow/internal/modules/passtypes"
	"github.com/alumasinde/passnow/internal/modules/roles"
	"github.com/alumasinde/passnow/internal/modules/sites"
	"github.com/alumasinde/passnow/internal/modules/users"
	"github.com/alumasinde/passnow/internal/modules/visitors"
	"github.com/alumasinde/passnow/internal/platform/audit"
	"github.com/alumasinde/passnow/internal/platform/httpx"
	"github.com/alumasinde/passnow/internal/platform/rbac"
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
		httpx.JSON(w, 200, map[string]any{"status": "ok", "service": "passnow"})
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
