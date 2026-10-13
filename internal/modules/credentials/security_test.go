package credentials

import (
	"context"
	"testing"
)

func TestVerifyRequiresAuthenticatedActorBeforeDatabaseAccess(t *testing.T) {
	s := NewService(nil, nil)
	if _, err := s.Verify(context.Background(), "credential-token"); err != ErrUnavailable {
		t.Fatalf("unauthenticated verification got %v, want ErrUnavailable", err)
	}
}

func TestCredentialHashIsDeterministicAndDistinct(t *testing.T) {
	a := hashToken("credential-A")
	if a != hashToken("credential-A") {
		t.Fatal("same token must produce the same hash")
	}
	if a == hashToken("credential-B") {
		t.Fatal("different tokens must not produce the same hash")
	}
	if len(a) != 64 {
		t.Fatalf("hash length=%d, want 64 hex characters", len(a))
	}
}
