package auth

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	UserID int64  `json:"uid"`
	OrgID  int64  `json:"oid"`
	Type   string `json:"typ"`
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
	ttl := s.AccessTTL
	t := "access"
	if refresh {
		ttl = s.RefreshTTL
		t = "refresh"
	}
	n := time.Now()
	c := Claims{uid, oid, t, jwt.RegisteredClaims{Issuer: s.Issuer, Subject: "user", IssuedAt: jwt.NewNumericDate(n), ExpiresAt: jwt.NewNumericDate(n.Add(ttl))}}
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
	if e != nil || c.Type != "access" {
		return nil, errors.New("invalid access token")
	}
	return c, nil
}
func (s *Service) Refresh(tok string) (string, error) {
	c, e := s.Parse(tok)
	if e != nil || c.Type != "refresh" {
		return "", errors.New("invalid refresh token")
	}
	return s.Issue(c.UserID, c.OrgID, false)
}
func With(ctx context.Context, c *Claims) context.Context { return context.WithValue(ctx, Key, c) }
func From(ctx context.Context) (*Claims, bool)            { c, ok := ctx.Value(Key).(*Claims); return c, ok }
