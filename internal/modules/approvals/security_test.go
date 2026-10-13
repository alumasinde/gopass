package approvals

import (
	"context"
	"testing"
)

func TestActRequiresAuthenticatedActorBeforeDatabaseAccess(t *testing.T) {
	s := NewService(nil, nil)
	if err := s.Act(context.Background(), 1, 10, true, "approve"); err != ErrForbidden {
		t.Fatalf("unauthenticated approval got %v, want ErrForbidden", err)
	}
}
