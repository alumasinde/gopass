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
	s := New("01234567890123456789012345678901", "gopass", time.Minute, time.Hour)
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
	if rc.SessionID == "" || c.SessionID == "" {
		t.Fatal("issued tokens must include a session ID")
	}
	if c.SessionID == rc.SessionID {
		t.Fatal("independent Issue calls must create separate sessions")
	}
	shared := "00112233445566778899aabbccddeeff"
	a, err := s.IssueInSession(10, 20, false, shared)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.IssueInSession(10, 20, true, shared)
	if err != nil {
		t.Fatal(err)
	}
	ac, err := s.Access(a)
	if err != nil {
		t.Fatal(err)
	}
	rc2, err := s.Parse(r)
	if err != nil {
		t.Fatal(err)
	}
	if ac.SessionID != shared || rc2.SessionID != shared {
		t.Fatal("access and refresh tokens must share the same session ID")
	}
	if _, err := s.Access(refresh); err == nil {
		t.Fatal("refresh token accepted as access token")
	}
}

func TestPlatformTokensAreIsolatedFromTenantTokens(t *testing.T) {
	s := New("01234567890123456789012345678901", "gopass", time.Minute, time.Hour)
	sid := "00112233445566778899aabbccddeeff"
	platformToken, err := s.IssuePlatformAccess(7, sid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Access(platformToken); err == nil {
		t.Fatal("platform token accepted by tenant access parser")
	}
	claims, err := s.PlatformAccess(platformToken)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 7 || claims.OrgID != 0 || claims.PrincipalType != "platform" {
		t.Fatalf("unexpected platform claims: %+v", claims)
	}
	tenantToken, err := s.IssueInSession(8, 9, false, sid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PlatformAccess(tenantToken); err == nil {
		t.Fatal("tenant token accepted by platform access parser")
	}
}
