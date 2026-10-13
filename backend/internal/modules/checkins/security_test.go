package checkins

import (
	"context"
	"testing"
)

func TestCreateRequiresAuthenticatedActorBeforeDatabaseAccess(t *testing.T) {
	s := NewService(nil, nil)
	if _, err := s.Create(context.Background(), 1, "credential-token", 2); err != ErrInvalid {
		t.Fatalf("unauthenticated check-in got %v, want ErrInvalid", err)
	}
}

func TestCredentialTokenHashIsDeterministicAndOneWaySized(t *testing.T) {
	first := hashToken("sample-credential")
	if first != hashToken("sample-credential") {
		t.Fatal("same credential must hash deterministically")
	}
	if first == hashToken("different-credential") {
		t.Fatal("different credentials must have different hashes")
	}
	if len(first) != 64 {
		t.Fatalf("SHA-256 hex hash length=%d, want 64", len(first))
	}
	if first == "sample-credential" {
		t.Fatal("raw credential must not be stored as hash")
	}
}
