package app

import (
	"context"
	"fmt"
	"github.com/alumasinde/passnow/internal/app/config"
	"github.com/alumasinde/passnow/internal/app/database"
	"github.com/alumasinde/passnow/internal/app/http/middleware"
	"github.com/alumasinde/passnow/internal/app/logger"
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
	"github.com/alumasinde/passnow/internal/platform/auth"
	"github.com/alumasinde/passnow/internal/platform/rbac"
	"github.com/alumasinde/passnow/internal/routes"
	"github.com/go-chi/chi/v5"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type App struct {
	db     interface{ Close() error }
	server *http.Server
	log    *slog.Logger
}

func New(cfg config.Config) (*App, error) {
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, e := database.Open(ctx, cfg.DBDSN)
	if e != nil {
		return nil, e
	}
	l := logger.New(cfg.LogLevel)
	tokens := auth.New(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTTL, cfg.RefreshTTL)
	az := rbac.New(db)
	au := audit.New(db, l)
	d := routes.Dependencies{Auth: users.NewAuthService(db, tokens), Authz: az, Audit: au, Users: users.NewHandler(db, az, au), Organizations: organizations.NewHandler(db, az, au), Roles: roles.NewHandler(db, az, au), Sites: sites.NewHandler(db, az, au), Gates: gates.NewHandler(db, az, au), Visitors: visitors.NewHandler(db, az, au), Gatepasses: gatepasses.NewHandler(db, az, au), Approvals: approvals.NewHandler(db, az, au), Credentials: credentials.NewHandler(db, az, au), Checkins: checkins.NewHandler(db, az, au), Checkouts: checkouts.NewHandler(db, az, au), PassTypes: passtypes.NewHandler(db, az, au)}
	r := chi.NewRouter()
	r.Use(middleware.Request, middleware.Security, middleware.CORS(cfg.CORS), middleware.Log(l), middleware.Recover(l))
	routes.RegisterAPI(r, d)
	return &App{db: db, server: &http.Server{Addr: fmt.Sprintf("%s:%d", cfg.Host, cfg.Port), Handler: r, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}, log: l}, nil
}
func (a *App) Run() error {
	ch := make(chan error, 1)
	go func() { a.log.Info("server starting", "addr", a.server.Addr); ch <- a.server.ListenAndServe() }()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case e := <-ch:
		if e == http.ErrServerClosed {
			return nil
		}
		return e
	case <-sig:
		ctx, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		e := a.server.Shutdown(ctx)
		if e != nil {
			return e
		}
		return a.db.Close()
	}
}
