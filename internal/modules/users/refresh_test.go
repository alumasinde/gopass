package users

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alumasinde/gopass/internal/platform/auth"
)

// refreshRepo is a concurrency-safe repository fake that models the atomic
// single-use property of RotateRefreshToken without requiring a MySQL server.
type refreshRepo struct {
	mu     sync.Mutex
	tokens map[string]bool
	user   *User
}

func newRefreshRepo() *refreshRepo {
	return &refreshRepo{tokens: make(map[string]bool), user: &User{ID: 7, OrganizationID: 3, IsActive: true}}
}
func (r *refreshRepo) Ping(context.Context) error                         { return nil }
func (r *refreshRepo) Create(context.Context, int64, *User, string) error { return nil }
func (r *refreshRepo) GetByID(_ context.Context, orgID, id int64) (*User, error) {
	if r.user != nil && r.user.ID == id && r.user.OrganizationID == orgID {
		u := *r.user
		return &u, nil
	}
	return nil, nil
}
func (r *refreshRepo) GetByEmail(context.Context, int64, string) (*User, error) { return nil, nil }
func (r *refreshRepo) List(context.Context, int64, int, int, string) ([]User, int64, error) {
	return nil, 0, nil
}
func (r *refreshRepo) Update(context.Context, int64, *User) error                     { return nil }
func (r *refreshRepo) Delete(context.Context, int64, int64) error                     { return nil }
func (r *refreshRepo) GetPasswordHash(context.Context, int64, string) (string, error) { return "", nil }
func (r *refreshRepo) UpdateLastLogin(context.Context, int64, int64) error            { return nil }
func (r *refreshRepo) StoreRefreshToken(_ context.Context, _, _ int64, hash string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[hash] = true
	return nil
}
func (r *refreshRepo) RotateRefreshToken(_ context.Context, oldHash string, _, _ int64, newHash string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.tokens[oldHash] {
		return errors.New("refresh token already consumed")
	}
	delete(r.tokens, oldHash)
	r.tokens[newHash] = true
	return nil
}
func (r *refreshRepo) RevokeRefreshToken(_ context.Context, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tokens, hash)
	return nil
}
func (r *refreshRepo) RevokeAllUserRefreshTokens(context.Context, int64) error { return nil }
func (r *refreshRepo) IsRefreshTokenValid(_ context.Context, hash string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tokens[hash], nil
}
func (r *refreshRepo) IsActive(context.Context, int64, int64) (bool, error) { return true, nil }

func TestRefreshTokenConcurrentUseOnlyOneSucceeds(t *testing.T) {
	repo := newRefreshRepo()
	tokens := auth.New("test-secret-that-is-long-enough-for-tests", "gopass-test", time.Minute, time.Hour)
	service := NewService(repo, tokens)
	old, err := tokens.Issue(7, 3, true)
	if err != nil {
		t.Fatal(err)
	}
	repo.tokens[hashToken(old)] = true

	const callers = 24
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			access, refresh, err := service.RefreshToken(context.Background(), old)
			if err == nil {
				if access == "" || refresh == "" {
					t.Errorf("successful refresh must return both tokens")
				}
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if successes != 1 {
		t.Fatalf("expected exactly one successful refresh, got %d", successes)
	}
}

func TestRefreshTokenReturnsRotatedPairAndInvalidatesOldToken(t *testing.T) {
	repo := newRefreshRepo()
	tokens := auth.New("test-secret-that-is-long-enough-for-tests", "gopass-test", time.Minute, time.Hour)
	service := NewService(repo, tokens)
	old, err := tokens.Issue(7, 3, true)
	if err != nil {
		t.Fatal(err)
	}
	repo.tokens[hashToken(old)] = true

	access, refresh, err := service.RefreshToken(context.Background(), old)
	if err != nil {
		t.Fatal(err)
	}
	if access == "" || refresh == "" {
		t.Fatal("expected both new tokens")
	}
	if _, err := tokens.Access(access); err != nil {
		t.Fatalf("new access token invalid: %v", err)
	}
	claims, err := tokens.Parse(refresh)
	if err != nil || claims.Type != "refresh" {
		t.Fatalf("new refresh token invalid: %v", err)
	}
	if repo.tokens[hashToken(old)] {
		t.Fatal("old refresh token should be consumed")
	}
	if !repo.tokens[hashToken(refresh)] {
		t.Fatal("new refresh token should be stored")
	}
}

func TestAuthHandlerInitializesLoginRateLimiter(t *testing.T) {
	tokens := auth.New("test-secret-that-is-long-enough-for-tests", "gopass-test", time.Minute, time.Hour)
	h := NewAuthService(nil, tokens)
	if h.rateLimiter == nil {
		t.Fatal("authentication handler must initialize its login rate limiter")
	}
	if got := clientIP("192.0.2.10:54321"); got != "192.0.2.10" {
		t.Fatalf("clientIP() = %q, want host without source port", got)
	}
}
