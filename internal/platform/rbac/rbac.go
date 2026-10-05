package rbac

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
)

const (
	ScopeOrganization = "ORGANIZATION"
	ScopeSite         = "SITE"
	ScopeGate         = "GATE"
)

type Service struct{ DB *sql.DB }

func New(db *sql.DB) *Service { return &Service{db} }

func (s *Service) Can(ctx context.Context, code string) (bool, error) {
	c, ok := auth.From(ctx)
	if !ok {
		return false, nil
	}
	org, e := tenancy.ID(ctx)
	if e != nil {
		return false, e
	}
	var n int
	e = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1`, c.UserID, org, code).Scan(&n)
	return n > 0, e
}

func (s *Service) CanWithScope(ctx context.Context, code, scopeType string, resourceID int64) (bool, error) {
	c, ok := auth.From(ctx)
	if !ok {
		return false, nil
	}
	org, e := tenancy.ID(ctx)
	if e != nil {
		return false, e
	}

	var n int
	var query string
	var args []interface{}

	switch scopeType {
	case ScopeOrganization:
		query = `SELECT COUNT(*) FROM user_roles ur JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1 AND ur.scope_type='ORGANIZATION'`
		args = []interface{}{c.UserID, org, code}
	case ScopeSite:
		query = `SELECT COUNT(*) FROM user_roles ur JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1 AND (ur.scope_type='ORGANIZATION' OR (ur.scope_type='SITE' AND ur.site_id=?))`
		args = []interface{}{c.UserID, org, code, resourceID}
	case ScopeGate:
		query = `SELECT COUNT(*) FROM user_roles ur JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1 AND (ur.scope_type='ORGANIZATION' OR (ur.scope_type='GATE' AND ur.gate_id=?))`
		args = []interface{}{c.UserID, org, code, resourceID}
	default:
		return false, fmt.Errorf("invalid scope type")
	}

	e = s.DB.QueryRowContext(ctx, query, args...).Scan(&n)
	return n > 0, e
}

func (s *Service) Require(ctx context.Context, code string) error {
	ok, e := s.Can(ctx, code)
	if e != nil {
		return e
	}
	if !ok {
		return fmt.Errorf("forbidden")
	}
	return nil
}

func (s *Service) RequireWithScope(ctx context.Context, code, scopeType string, resourceID int64) error {
	ok, e := s.CanWithScope(ctx, code, scopeType, resourceID)
	if e != nil {
		return e
	}
	if !ok {
		return fmt.Errorf("forbidden")
	}
	return nil
}

func (s *Service) GetUserPermissions(ctx context.Context, userID, orgID int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT p.code FROM user_roles ur JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND ur.is_active=1`, userID, orgID)
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
	return codes, nil
}

func (s *Service) HasAnyPermission(ctx context.Context, codes []string) (bool, error) {
	if len(codes) == 0 {
		return false, nil
	}
	c, ok := auth.From(ctx)
	if !ok {
		return false, nil
	}
	org, e := tenancy.ID(ctx)
	if e != nil {
		return false, e
	}

	args := make([]interface{}, 0, len(codes)+2)
	args = append(args, c.UserID, org)
	placeholders := make([]string, len(codes))
	for i, code := range codes {
		placeholders[i] = "?"
		args = append(args, code)
	}

	query := fmt.Sprintf(`SELECT COUNT(*) FROM user_roles ur JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code IN (%s) AND ur.is_active=1`, fmt.Sprintf("%s", placeholders))
	var n int
	e = s.DB.QueryRowContext(ctx, query, args...).Scan(&n)
	return n > 0, e
}
