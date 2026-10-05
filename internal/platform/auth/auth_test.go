package auth

import (
	"testing"
	"time"
)

func TestPasswordHashAndCheck(t *testing.T) {
	hash, err := Hash("StrongPassword123!")
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(hash, "StrongPassword123!"); err != nil {
		t.Fatal("valid password rejected")
	}
	if err := Check(hash, "WrongPassword123!"); err == nil {
		t.Fatal("invalid password accepted")
	}
}

func TestAccessAndRefreshTokens(t *testing.T) {
	s := New("01234567890123456789012345678901", "passnow", time.Minute, time.Hour)
	access, err := s.Issue(10, 20, false)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Access(access)
	if err != nil {
		t.Fatal(err)
	}
	if c.UserID != 10 || c.OrgID != 20 || c.Type != "access" {
		t.Fatalf("unexpected claims: %+v", c)
	}
	refresh, err := s.Issue(10, 20, true)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := s.Parse(refresh)
	if err != nil {
		t.Fatal(err)
	}
	if rc.Type != "refresh" {
		t.Fatalf("expected refresh token, got %s", rc.Type)
	}
	if _, err := s.Access(refresh); err == nil {
		t.Fatal("refresh token accepted as access token")
	}
}
