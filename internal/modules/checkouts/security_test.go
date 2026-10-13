package checkouts

import (
	"context"
	"testing"
)

func TestCreateRequiresAuthenticatedActorBeforeDatabaseAccess(t *testing.T) {
	s := NewService(nil, nil)
	if _, err := s.Create(context.Background(), 1, 10, 2); err != ErrInvalid {
		t.Fatalf("unauthenticated check-out got %v, want ErrInvalid", err)
	}
}
