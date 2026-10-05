package middleware

import (
	"context"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/httpx"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
	"github.com/google/uuid"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type key string

const RequestID key = "request_id"

func Request(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), RequestID, id)))
	})
}
func Security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
func CORS(origins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			o := r.Header.Get("Origin")
			ok := false
			for _, x := range origins {
				if x == o {
					ok = true
				}
			}
			if ok {
				w.Header().Set("Access-Control-Allow-Origin", o)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			}
			if r.Method == http.MethodOptions {
				if ok {
					w.WriteHeader(204)
				} else {
					w.WriteHeader(403)
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
func Recover(l *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					l.ErrorContext(r.Context(), "panic recovered", "error", v)
					httpx.Error(w, 500, "internal_error", "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
func Log(l *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			st := time.Now()
			rw := &sw{w, 200}
			next.ServeHTTP(rw, r)
			l.InfoContext(r.Context(), "http request", "method", r.Method, "path", r.URL.Path, "status", rw.status, "duration_ms", time.Since(st).Milliseconds())
		})
	}
}

type sw struct {
	http.ResponseWriter
	status int
}

func (w *sw) WriteHeader(s int) { w.status = s; w.ResponseWriter.WriteHeader(s) }
func Bearer(a *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := strings.Fields(r.Header.Get("Authorization"))
			if len(p) != 2 || strings.ToLower(p[0]) != "bearer" {
				httpx.Error(w, 401, "unauthorized", "authentication required")
				return
			}
			c, e := a.Access(p[1])
			if e != nil {
				httpx.Error(w, 401, "invalid_token", "invalid or expired access token")
				return
			}
			ctx := auth.With(r.Context(), c)
			ctx = tenancy.With(ctx, c.OrgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
