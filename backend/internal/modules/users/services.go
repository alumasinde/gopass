package users

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
)

var (
	ErrUserNotFound        = errors.New("user not found")
	ErrInvalidPassword     = errors.New("invalid password")
	ErrUserAlreadyExists   = errors.New("user already exists")
	ErrInvalidEmail        = errors.New("invalid email")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
)

type Service struct {
	repo Repository
	auth *auth.Service
}

func NewService(repo Repository, auth *auth.Service) *Service {
	return &Service{repo: repo, auth: auth}
}

func (s *Service) CreateUser(ctx context.Context, orgID int64, firstName, lastName, email, password string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !isValidEmail(email) {
		return nil, ErrInvalidEmail
	}
	if len(password) < 12 {
		return nil, ErrInvalidPassword
	}

	existing, _ := s.repo.GetByEmail(ctx, orgID, email)
	if existing != nil {
		return nil, ErrUserAlreadyExists
	}

	hash, err := auth.Hash(password)
	if err != nil {
		return nil, err
	}

	user := &User{
		FirstName: strings.TrimSpace(firstName),
		LastName:  strings.TrimSpace(lastName),
		Email:     email,
		IsActive:  true,
	}

	if err := s.repo.Create(ctx, orgID, user, hash); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *Service) GetUser(ctx context.Context, orgID, id int64) (*User, error) {
	user, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func (s *Service) ListUsers(ctx context.Context, orgID int64, offset, limit int, search string) ([]User, int64, error) {
	return s.repo.List(ctx, orgID, offset, limit, search)
}

func (s *Service) UpdateUser(ctx context.Context, orgID int64, user *User) error {
	existing, err := s.repo.GetByID(ctx, orgID, user.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrUserNotFound
	}

	user.Email = strings.ToLower(strings.TrimSpace(user.Email))
	user.FirstName = strings.TrimSpace(user.FirstName)
	user.LastName = strings.TrimSpace(user.LastName)

	return s.repo.Update(ctx, orgID, user)
}

func (s *Service) DeleteUser(ctx context.Context, orgID, id int64) error {
	existing, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrUserNotFound
	}
	return s.repo.Delete(ctx, orgID, id)
}

func (s *Service) Authenticate(ctx context.Context, orgID int64, email, password string) (*User, string, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	hash, err := s.repo.GetPasswordHash(ctx, orgID, email)
	if err != nil {
		return nil, "", "", err
	}
	if hash == "" {
		// Timing attack prevention: always do a dummy bcrypt compare
		// even if user doesn't exist
		dummyHash, _ := auth.Hash("dummy-password-for-timing")
		_ = auth.Check(dummyHash, password)
		return nil, "", "", ErrUserNotFound
	}

	if err := auth.Check(hash, password); err != nil {
		return nil, "", "", ErrInvalidPassword
	}

	user, err := s.repo.GetByEmail(ctx, orgID, email)
	if err != nil {
		return nil, "", "", err
	}
	if user == nil || !user.IsActive {
		return nil, "", "", ErrUserNotFound
	}

	if err := s.repo.UpdateLastLogin(ctx, orgID, user.ID); err != nil {
		return nil, "", "", err
	}

	sessionID, err := auth.NewSessionID()
	if err != nil {
		return nil, "", "", err
	}
	accessToken, err := s.auth.IssueInSession(user.ID, orgID, false, sessionID)
	if err != nil {
		return nil, "", "", err
	}

	refreshToken, err := s.auth.IssueInSession(user.ID, orgID, true, sessionID)
	if err != nil {
		return nil, "", "", err
	}

	// Store hashed refresh token
	tokenHash := hashToken(refreshToken)
	if err := s.repo.StoreRefreshToken(ctx, user.ID, orgID, sessionID, tokenHash, time.Now().UTC().Add(s.auth.RefreshTTL)); err != nil {
		return nil, "", "", err
	}

	return user, accessToken, refreshToken, nil
}

func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (string, string, error) {
	claims, err := s.auth.Parse(refreshToken)
	if err != nil || claims.Type != "refresh" {
		return "", "", ErrInvalidRefreshToken
	}
	ctx = tenancy.With(ctx, claims.OrgID)

	user, err := s.repo.GetByID(ctx, claims.OrgID, claims.UserID)
	if err != nil {
		return "", "", err
	}
	if user == nil || !user.IsActive {
		return "", "", ErrUserNotFound
	}
	active, err := s.repo.IsActive(ctx, claims.UserID, claims.OrgID)
	if err != nil {
		return "", "", err
	}
	if !active {
		return "", "", ErrUserNotFound
	}
	if claims.SessionID == "" {
		return "", "", ErrInvalidRefreshToken
	}
	newAccessToken, err := s.auth.IssueInSession(claims.UserID, claims.OrgID, false, claims.SessionID)
	if err != nil {
		return "", "", err
	}
	newRefreshToken, err := s.auth.IssueInSession(claims.UserID, claims.OrgID, true, claims.SessionID)
	if err != nil {
		return "", "", err
	}

	// The repository consumes the old token and inserts the replacement in one
	// transaction. Exactly one concurrent request can consume the old token.
	oldHash := hashToken(refreshToken)
	newHash := hashToken(newRefreshToken)
	expiresAt := time.Now().UTC().Add(s.auth.RefreshTTL)
	if err := s.repo.RotateRefreshToken(ctx, oldHash, claims.UserID, claims.OrgID, claims.SessionID, newHash, expiresAt); err != nil {
		return "", "", err
	}
	return newAccessToken, newRefreshToken, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	claims, err := s.auth.Parse(refreshToken)
	if err != nil || claims.Type != "refresh" || claims.SessionID == "" {
		return ErrInvalidRefreshToken
	}
	ctx = tenancy.With(ctx, claims.OrgID)
	return s.repo.RevokeSession(ctx, claims.SessionID, claims.UserID, claims.OrgID, hashToken(refreshToken))
}

func (s *Service) LogoutAll(ctx context.Context, userID int64) error {
	return s.repo.RevokeAllUserRefreshTokens(ctx, userID)
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (s *Service) IsActive(ctx context.Context, userID, orgID int64) (bool, error) {
	return s.repo.IsActive(ctx, userID, orgID)
}

func (s *Service) IsSessionActive(ctx context.Context, sessionID string, userID, orgID int64) (bool, error) {
	active, err := s.repo.IsActive(ctx, userID, orgID)
	if err != nil || !active {
		return false, err
	}
	return s.repo.IsSessionActive(ctx, sessionID, userID, orgID)
}

func isValidEmail(email string) bool {
	return strings.Contains(email, "@") && strings.Contains(email, ".")
}
