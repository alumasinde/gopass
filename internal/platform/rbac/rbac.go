package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
)

const (
	ScopeOrganization = "ORGANIZATION"
	ScopeSite         = "SITE"
	ScopeGate         = "GATE"
	ScopeOwn          = "OWN"
)

var ErrForbidden = errors.New("rbac: forbidden")

type Service struct{ DB *sql.DB }

func New(db *sql.DB) *Service { return &Service{DB: db} }

func (s *Service) actor(ctx context.Context) (*auth.Claims, int64, error) {
	c, ok := auth.From(ctx)
	if !ok {
		return nil, 0, fmt.Errorf("authentication required")
	}
	org, err := tenancy.ID(ctx)
	if err != nil {
		return nil, 0, err
	}
	if c.OrgID != org {
		return nil, 0, fmt.Errorf("organization mismatch")
	}
	return c, org, nil
}

func (s *Service) Can(ctx context.Context, code string) (bool, error) {
	c, org, err := s.actor(ctx)
	if err != nil {
		return false, nil
	}
	var n int
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1`, c.UserID, org, strings.TrimSpace(code)).Scan(&n)
	return n > 0, err
}

func (s *Service) CanWithScope(ctx context.Context, code, scopeType string, resourceID int64) (bool, error) {
	c, org, err := s.actor(ctx)
	if err != nil {
		return false, nil
	}
	code = strings.TrimSpace(code)
	switch scopeType {
	case ScopeOrganization:
		var n int
		err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1 AND ur.scope_type='ORGANIZATION'`, c.UserID, org, code).Scan(&n)
		return n > 0, err
	case ScopeSite:
		var n int
		err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1 AND (ur.scope_type='ORGANIZATION' OR (ur.scope_type='SITE' AND ur.site_id=?))`, c.UserID, org, code, resourceID).Scan(&n)
		return n > 0, err
	case ScopeGate:
		var n int
		err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1 AND (ur.scope_type='ORGANIZATION' OR (ur.scope_type='GATE' AND ur.gate_id=?))`, c.UserID, org, code, resourceID).Scan(&n)
		return n > 0, err
	case ScopeOwn:
		var n int
		err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1 AND ur.scope_type='OWN' AND ur.user_id=?`, c.UserID, org, code, resourceID).Scan(&n)
		return n > 0, err
	default:
		return false, fmt.Errorf("invalid scope type")
	}
}

func (s *Service) Require(ctx context.Context, code string) error {
	ok, err := s.Can(ctx, code)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}
func (s *Service) RequireWithScope(ctx context.Context, code, scopeType string, resourceID int64) error {
	ok, err := s.CanWithScope(ctx, code, scopeType, resourceID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}

func (s *Service) GetUserPermissions(ctx context.Context, userID, orgID int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT p.code FROM user_roles ur JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND ur.is_active=1 ORDER BY p.code`, userID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, rows.Err()
}
func (s *Service) HasAnyPermission(ctx context.Context, codes []string) (bool, error) {
	if len(codes) == 0 {
		return false, nil
	}
	c, org, err := s.actor(ctx)
	if err != nil {
		return false, nil
	}
	ph := make([]string, len(codes))
	args := []any{c.UserID, org}
	for i, v := range codes {
		ph[i] = "?"
		args = append(args, v)
	}
	var n int
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code IN (`+strings.Join(ph, ",")+`) AND ur.is_active=1`, args...).Scan(&n)
	return n > 0, err
}
