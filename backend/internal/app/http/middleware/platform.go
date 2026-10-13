package middleware

import (
	"database/sql"
	"github.com/alumasinde/gopass/internal/platform/apperr"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/httpx"
	"net/http"
	"strings"
)

// PlatformBearer accepts only platform-principal tokens backed by an active,
// unexpired platform session. Tenant user tokens can never enter control-plane routes.
func PlatformBearer(tokens *auth.Service, db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := strings.Fields(r.Header.Get("Authorization"))
			if len(p) != 2 || !strings.EqualFold(p[0], "Bearer") {
				httpx.Fail(w, apperr.Unauthorized)
				return
			}
			claims, err := tokens.PlatformAccess(p[1])
			if err != nil {
				httpx.Fail(w, apperr.InvalidToken)
				return
			}
			var active bool
			err = db.QueryRowContext(r.Context(), `SELECT pu.is_active FROM platform_users pu JOIN platform_sessions ps ON ps.platform_user_id=pu.id WHERE pu.id=? AND ps.session_id=? AND ps.revoked_at IS NULL AND ps.expires_at>UTC_TIMESTAMP()`, claims.UserID, claims.SessionID).Scan(&active)
			if err != nil || !active {
				httpx.Fail(w, apperr.Unauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(auth.With(r.Context(), claims)))
		})
	}
}
