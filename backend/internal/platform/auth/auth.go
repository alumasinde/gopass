package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	UserID        int64  `json:"uid"`
	OrgID         int64  `json:"oid"`
	Type          string `json:"typ"`
	SessionID     string `json:"sid"`
	PrincipalType string `json:"prn"`
	jwt.RegisteredClaims
}
type key string

const Key key = "claims"

type Service struct {
	Secret, Issuer        string
	AccessTTL, RefreshTTL time.Duration
}

func New(sec, iss string, a, r time.Duration) *Service { return &Service{sec, iss, a, r} }
func Hash(p string) (string, error) {
	b, e := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	return string(b), e
}
func Check(h, p string) error { return bcrypt.CompareHashAndPassword([]byte(h), []byte(p)) }
func (s *Service) Issue(uid, oid int64, refresh bool) (string, error) {
	sessionID, err := NewSessionID()
	if err != nil {
		return "", err
	}
	return s.IssueInSession(uid, oid, refresh, sessionID)
}

// NewSessionID creates a cryptographically random identifier shared by the
// access and refresh tokens belonging to one login session.
func NewSessionID() (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(nonce[:]), nil
}

func (s *Service) IssueInSession(uid, oid int64, refresh bool, sessionID string) (string, error) {
	if uid < 1 || oid < 1 || sessionID == "" {
		return "", errors.New("invalid token subject or session")
	}
	ttl := s.AccessTTL
	t := "access"
	if refresh {
		ttl = s.RefreshTTL
		t = "refresh"
	}
	n := time.Now()
	// Each JWT has a unique jti, while sid binds access and refresh tokens to
	// the same revocable server-side session.
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	c := Claims{UserID: uid, OrgID: oid, Type: t, SessionID: sessionID, PrincipalType: "user", RegisteredClaims: jwt.RegisteredClaims{Issuer: s.Issuer, Subject: "user", ID: hex.EncodeToString(nonce[:]), IssuedAt: jwt.NewNumericDate(n), ExpiresAt: jwt.NewNumericDate(n.Add(ttl))}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(s.Secret))
}
func (s *Service) Parse(tok string) (*Claims, error) {
	t, e := jwt.ParseWithClaims(tok, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("bad signing method")
		}
		return []byte(s.Secret), nil
	}, jwt.WithIssuer(s.Issuer))
	if e != nil {
		return nil, e
	}
	c, ok := t.Claims.(*Claims)
	if !ok || !t.Valid {
		return nil, errors.New("invalid token")
	}
	return c, nil
}
func (s *Service) Access(tok string) (*Claims, error) {
	c, e := s.Parse(tok)
	if e != nil || c.Type != "access" || c.SessionID == "" || c.PrincipalType != "user" {
		return nil, errors.New("invalid access token")
	}
	return c, nil
}

// IssuePlatformAccess issues a short-lived platform-administrator token. Platform
// principals deliberately have no organization ID and are accepted only by the
// dedicated platform middleware.
func (s *Service) IssuePlatformAccess(uid int64, sessionID string) (string, error) {
	if uid < 1 || sessionID == "" {
		return "", errors.New("invalid platform subject or session")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	now := time.Now()
	c := Claims{UserID: uid, OrgID: 0, Type: "access", SessionID: sessionID, PrincipalType: "platform", RegisteredClaims: jwt.RegisteredClaims{Issuer: s.Issuer, Subject: "platform_user", ID: hex.EncodeToString(nonce[:]), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(s.AccessTTL))}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(s.Secret))
}
func (s *Service) PlatformAccess(tok string) (*Claims, error) {
	c, err := s.Parse(tok)
	if err != nil || c.Type != "access" || c.PrincipalType != "platform" || c.UserID < 1 || c.OrgID != 0 || c.SessionID == "" {
		return nil, errors.New("invalid platform access token")
	}
	return c, nil
}

func With(ctx context.Context, c *Claims) context.Context { return context.WithValue(ctx, Key, c) }
func From(ctx context.Context) (*Claims, bool)            { c, ok := ctx.Value(Key).(*Claims); return c, ok }
