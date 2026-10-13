package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alumasinde/gopass/internal/platform/auth"
)

type checkerStub struct {
	active        bool
	sessionActive bool
	userErr       error
	sessionErr    error
}

func (c checkerStub) IsActive(context.Context, int64, int64) (bool, error) {
	return c.active, c.userErr
}
func (c checkerStub) IsSessionActive(context.Context, string, int64, int64) (bool, error) {
	return c.sessionActive, c.sessionErr
}

func TestBearerRejectsMissingAndMalformedAuthorization(t *testing.T) {
	a := auth.New("test-secret-that-is-long-enough-for-tests", "gopass-test", time.Minute, time.Hour)
	mw := Bearer(a, checkerStub{active: true, sessionActive: true})
	called := false
	h := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	for _, header := range []string{"", "Basic abc", "Bearer", "Bearer one two"} {
		r := httptest.NewRequest(http.MethodGet, "/protected", nil)
		r.Header.Set("Authorization", header)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("header %q: status=%d, want 401", header, w.Code)
		}
	}
	if called {
		t.Fatal("protected handler must not run for malformed authorization")
	}
}

func TestBearerFailsClosedForRevokedUserOrSession(t *testing.T) {
	a := auth.New("test-secret-that-is-long-enough-for-tests", "gopass-test", time.Minute, time.Hour)
	token, err := a.Issue(7, 3, false)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		checker checkerStub
	}{
		{"inactive user", checkerStub{active: false, sessionActive: true}},
		{"revoked session", checkerStub{active: true, sessionActive: false}},
		{"user lookup error", checkerStub{userErr: context.DeadlineExceeded}},
		{"session lookup error", checkerStub{active: true, sessionErr: context.DeadlineExceeded}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			h := Bearer(a, tc.checker)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
			r := httptest.NewRequest(http.MethodGet, "/protected", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d, want 401", w.Code)
			}
			if called {
				t.Fatal("protected handler ran despite failed security check")
			}
		})
	}
}

func TestBearerAllowsActiveSessionAndAddsClaims(t *testing.T) {
	a := auth.New("test-secret-that-is-long-enough-for-tests", "gopass-test", time.Minute, time.Hour)
	token, err := a.Issue(7, 3, false)
	if err != nil {
		t.Fatal(err)
	}
	h := Bearer(a, checkerStub{active: true, sessionActive: true})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.From(r.Context())
		if !ok || claims.UserID != 7 || claims.OrgID != 3 {
			t.Errorf("claims missing or incorrect: %#v", claims)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodGet, "/protected", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204", w.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := Security(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	for header, want := range map[string]string{"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY", "Referrer-Policy": "no-referrer", "Cache-Control": "no-store"} {
		if got := w.Header().Get(header); got != want {
			t.Errorf("%s=%q, want %q", header, got, want)
		}
	}
}

func TestCORSRejectsUnlistedOriginPreflight(t *testing.T) {
	h := CORS([]string{"https://trusted.example"})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("handler must not run for preflight") }))
	r := httptest.NewRequest(http.MethodOptions, "/api", nil)
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("untrusted origin was reflected: %q", got)
	}
}

func TestCORSAllowsTenantLocalhostSubdomainPattern(t *testing.T) {
	h := CORS([]string{"http://localhost:5173", "http://*.localhost:5173"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodOptions, "/api", nil)
	r.Header.Set("Origin", "http://acme-security.localhost:5173")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://acme-security.localhost:5173" {
		t.Fatalf("allow origin=%q", got)
	}
}

func TestCORSWildcardDoesNotAllowNestedOrLookalikeHosts(t *testing.T) {
	for _, origin := range []string{
		"http://evil.acme.localhost:5173",
		"http://acme.localhost.evil.example:5173",
		"http://localhost:5173",
	} {
		if originMatches("http://*.localhost:5173", origin) {
			t.Errorf("wildcard unexpectedly matched %q", origin)
		}
	}
}
