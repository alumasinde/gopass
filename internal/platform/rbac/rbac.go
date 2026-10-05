package rbac

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
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
