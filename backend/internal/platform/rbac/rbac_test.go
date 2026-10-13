package rbac

import (
	"context"
	"testing"

	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
)

func TestCanFailsClosedWithoutAuthenticatedActor(t *testing.T) {
	s := New(nil)
	allowed, err := s.Can(context.Background(), "users.read")
	if err != nil {
		t.Fatalf("expected deny without DB access, got error: %v", err)
	}
	if allowed {
		t.Fatal("unauthenticated context was granted permission")
	}
}

func TestCanFailsClosedWhenClaimOrganizationDiffersFromContext(t *testing.T) {
	ctx := auth.With(context.Background(), &auth.Claims{UserID: 9, OrgID: 100})
	ctx = tenancy.With(ctx, 200)
	allowed, err := New(nil).Can(ctx, "users.read")
	if err != nil {
		t.Fatalf("expected deny without DB access, got error: %v", err)
	}
	if allowed {
		t.Fatal("organization-mismatched claims were granted permission")
	}
}

func TestCanWithScopeRejectsInvalidScopeAndIDs(t *testing.T) {
	ctx := auth.With(context.Background(), &auth.Claims{UserID: 9, OrgID: 100})
	ctx = tenancy.With(ctx, 100)
	s := New(nil)
	cases := []struct {
		name, scope string
		id          int64
	}{
		{"unknown scope", "GLOBAL", 1},
		{"site id zero", ScopeSite, 0},
		{"site id negative", ScopeSite, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			allowed, err := s.CanWithScope(ctx, "users.read", tc.scope, tc.id)
			if allowed {
				t.Fatal("invalid scope was granted")
			}
			if tc.scope == "GLOBAL" && err == nil {
				t.Fatal("unknown scope should return an error")
			}
			if tc.scope != "GLOBAL" && err != nil {
				t.Fatalf("invalid resource ID should deny without DB access, got %v", err)
			}
		})
	}
}

func TestRequireReturnsForbiddenWhenNoPermissionCanBeEstablished(t *testing.T) {
	err := New(nil).Require(context.Background(), "users.delete")
	if err != ErrForbidden {
		t.Fatalf("got %v, want ErrForbidden", err)
	}
}
