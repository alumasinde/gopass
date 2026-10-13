package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alumasinde/gopass/internal/app/config"
	"github.com/alumasinde/gopass/internal/app/database"
	"github.com/alumasinde/gopass/internal/app/http/middleware"
	"github.com/alumasinde/gopass/internal/app/logger"
	"github.com/alumasinde/gopass/internal/modules/approvals"
	"github.com/alumasinde/gopass/internal/modules/checkins"
	"github.com/alumasinde/gopass/internal/modules/checkouts"
	"github.com/alumasinde/gopass/internal/modules/credentials"
	"github.com/alumasinde/gopass/internal/modules/gatepasses"
	"github.com/alumasinde/gopass/internal/modules/gates"
	"github.com/alumasinde/gopass/internal/modules/notifications"
	"github.com/alumasinde/gopass/internal/modules/organizations"
	"github.com/alumasinde/gopass/internal/modules/passtypes"
	"github.com/alumasinde/gopass/internal/modules/roles"
	"github.com/alumasinde/gopass/internal/modules/sites"
	"github.com/alumasinde/gopass/internal/modules/users"
	"github.com/alumasinde/gopass/internal/modules/visitors"
	"github.com/alumasinde/gopass/internal/platform/audit"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/platformadmin"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"github.com/alumasinde/gopass/internal/routes"
	"github.com/go-chi/chi/v5"
)

type App struct {
	db                 interface{ Close() error }
	server             *http.Server
	log                *slog.Logger
	notificationWorker *notifications.Worker
	tenantProvisioner  *platformadmin.Provisioner
}

func New(cfg config.Config) (*App, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sharedDB, e := database.Open(ctx, cfg.DBDSN)
	if e != nil {
		return nil, fmt.Errorf("open shared tenant database: %w", e)
	}
	platformDB := sharedDB
	if cfg.PlatformDBDSN != cfg.DBDSN {
		platformDB, e = database.Open(ctx, cfg.PlatformDBDSN)
		if e != nil {
			_ = sharedDB.Close()
			return nil, fmt.Errorf("open platform control-plane database: %w", e)
		}
	}
	router := database.NewRouter(platformDB, sharedDB)
	l := logger.New(cfg.LogLevel)
	tokens := auth.New(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTTL, cfg.RefreshTTL)
	az := rbac.New(router)
	au := audit.New(router, l)
	authHandler := users.NewAuthService(router, tokens, au)
	authHandler.SetPlatformDB(platformDB)
	usersHandler := users.NewHandler(router, az, au)
	d := routes.Dependencies{Auth: authHandler, Authz: az, Audit: au, Users: usersHandler, Organizations: organizations.NewHandler(router, az, au), Roles: roles.NewHandler(router, az, au), Sites: sites.NewHandler(router, az, au), Gates: gates.NewHandler(router, az, au), Visitors: visitors.NewHandler(router, az, au), Gatepasses: gatepasses.NewHandler(router, az, au), Approvals: approvals.NewHandler(router, az, au), Credentials: credentials.NewHandler(router, az, au), Checkins: checkins.NewHandler(router, az, au), Checkouts: checkouts.NewHandler(router, az, au), PassTypes: passtypes.NewHandler(router, az, au), Notifications: notifications.NewHandler(router, az), Platform: platformadmin.New(platformDB, tokens, router)}
	r := chi.NewRouter()
	//r.Use(middleware.Request, middleware.Security, middleware.CORS(cfg.CORS), middleware.Log(l), middleware.Recover(l), middleware.Bearer(tokens, authHandler.Service))
	r.Use(middleware.Request, middleware.Security, middleware.CORS(cfg.CORS), middleware.Log(l), middleware.Recover(l))
	routes.RegisterAPI(r, d)
	smtpCfg := notifications.SMTPConfig{Host: os.Getenv("SMTP_HOST"), Port: os.Getenv("SMTP_PORT"), Username: os.Getenv("SMTP_USERNAME"), Password: os.Getenv("SMTP_PASSWORD"), From: os.Getenv("SMTP_FROM")}
	return &App{db: router, server: &http.Server{Addr: fmt.Sprintf("%s:%d", cfg.Host, cfg.Port), Handler: r, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}, log: l, notificationWorker: notifications.NewWorker(router, smtpCfg, l), tenantProvisioner: platformadmin.NewProvisioner(platformDB, l, router)}, nil
}
func (a *App) Run() error {
	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	go a.notificationWorker.Run(workerCtx)
	go a.tenantProvisioner.Run(workerCtx)
	ch := make(chan error, 1)
	go func() { a.log.Info("server starting", "addr", a.server.Addr); ch <- a.server.ListenAndServe() }()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case e := <-ch:
		stopWorkers()
		if e == http.ErrServerClosed {
			return nil
		}
		return e
	case <-sig:
		stopWorkers()
		ctx, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		e := a.server.Shutdown(ctx)
		if e != nil {
			return e
		}
		return a.db.Close()
	}
}
