package tenancy

import (
	"context"
	"testing"
)

func TestOrganizationContext(t *testing.T) {
	ctx := With(context.Background(), 42)
	id, err := ID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if id != 42 {
		t.Fatalf("got %d", id)
	}
}
func TestMissingOrganizationContext(t *testing.T) {
	if _, err := ID(context.Background()); err == nil {
		t.Fatal("expected missing tenant error")
	}
}
