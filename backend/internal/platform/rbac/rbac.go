package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/alumasinde/gopass/internal/app/database"

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

type Service struct{ DB database.Handle }

func New(db any) *Service { return &Service{DB: database.Adapt(db)} }

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
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN users u ON u.id=ur.user_id AND u.organization_id=ur.organization_id JOIN roles r ON r.id=ur.role_id AND r.organization_id=ur.organization_id JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1 AND ur.scope_type='ORGANIZATION'`, c.UserID, org, strings.TrimSpace(code)).Scan(&n)
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
		err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN users u ON u.id=ur.user_id AND u.organization_id=ur.organization_id JOIN roles r ON r.id=ur.role_id AND r.organization_id=ur.organization_id JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1 AND ur.scope_type='ORGANIZATION'`, c.UserID, org, code).Scan(&n)
		return n > 0, err
	case ScopeSite:
		if resourceID < 1 {
			return false, nil
		}
		var exists int
		if err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sites WHERE id=? AND organization_id=? AND is_active=1`, resourceID, org).Scan(&exists); err != nil || exists != 1 {
			return false, err
		}
		var n int
		query := `SELECT COUNT(*) FROM user_roles ur JOIN users u ON u.id=ur.user_id AND u.organization_id=ur.organization_id JOIN roles r ON r.id=ur.role_id AND r.organization_id=ur.organization_id JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1 AND (ur.scope_type='ORGANIZATION' OR (ur.scope_type='SITE' AND ur.site_id=?`
		args := []any{c.UserID, org, code, resourceID}
		// A gate-scoped grant may read its parent site metadata, but it never
		// grants site-management rights such as update/delete/create.
		if code == "sites.view" {
			query += ` OR (ur.scope_type='GATE' AND ur.gate_id IN (SELECT g.id FROM gates g WHERE g.organization_id=ur.organization_id AND g.site_id=? AND g.is_active=1))`
			args = append(args, resourceID)
		}
		query += `))`
		err = s.DB.QueryRowContext(ctx, query, args...).Scan(&n)
		return n > 0, err
	case ScopeGate:
		if resourceID < 1 {
			return false, nil
		}
		// Validate the target gate and its parent site in the caller's tenant.
		// This prevents a forged gate ID or inactive parent site from becoming
		// an authorization oracle across tenants.
		var siteID int64
		err = s.DB.QueryRowContext(ctx, `SELECT g.site_id FROM gates g JOIN sites s ON s.id=g.site_id AND s.organization_id=g.organization_id WHERE g.id=? AND g.organization_id=? AND g.is_active=1 AND s.is_active=1`, resourceID, org).Scan(&siteID)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		var n int
		err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN users u ON u.id=ur.user_id AND u.organization_id=ur.organization_id JOIN roles r ON r.id=ur.role_id AND r.organization_id=ur.organization_id JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1 AND (ur.scope_type='ORGANIZATION' OR (ur.scope_type='SITE' AND ur.site_id=?) OR (ur.scope_type='GATE' AND ur.gate_id=?))`, c.UserID, org, code, siteID, resourceID).Scan(&n)
		return n > 0, err
	case ScopeOwn:
		var n int
		err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN users u ON u.id=ur.user_id AND u.organization_id=ur.organization_id JOIN roles r ON r.id=ur.role_id AND r.organization_id=ur.organization_id JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1 AND ur.scope_type='OWN' AND ur.user_id=?`, c.UserID, org, code, resourceID).Scan(&n)
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
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT p.code FROM user_roles ur JOIN users u ON u.id=ur.user_id AND u.organization_id=ur.organization_id JOIN roles r ON r.id=ur.role_id AND r.organization_id=ur.organization_id JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND ur.is_active=1 AND ur.scope_type='ORGANIZATION' ORDER BY p.code`, userID, orgID)
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
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN users u ON u.id=ur.user_id AND u.organization_id=ur.organization_id JOIN roles r ON r.id=ur.role_id AND r.organization_id=ur.organization_id JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code IN (`+strings.Join(ph, ",")+`) AND ur.is_active=1 AND ur.scope_type='ORGANIZATION'`, args...).Scan(&n)
	return n > 0, err
}

// HasPermissionAnyScope reports whether a permission is assigned at any active
// scope. It is used to distinguish a legitimate empty scoped list from a user
// who has no permission at all.
func (s *Service) HasPermissionAnyScope(ctx context.Context, code string) (bool, error) {
	c, org, err := s.actor(ctx)
	if err != nil {
		return false, nil
	}
	var n int
	err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN users u ON u.id=ur.user_id AND u.organization_id=ur.organization_id JOIN roles r ON r.id=ur.role_id AND r.organization_id=ur.organization_id JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1`, c.UserID, org, strings.TrimSpace(code)).Scan(&n)
	return n > 0, err
}

// AccessibleSites returns the active site IDs visible to the caller for a
// permission. Organization-wide grants set all=true; scoped grants return only
// the explicitly assigned active sites. An empty result is not an error and
// must be returned as an empty list rather than falling back to organization scope.
func (s *Service) AccessibleSites(ctx context.Context, code string) (all bool, ids []int64, err error) {
	c, org, err := s.actor(ctx)
	if err != nil {
		return false, nil, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT ur.scope_type,ur.site_id,ur.gate_id FROM user_roles ur JOIN users u ON u.id=ur.user_id AND u.organization_id=ur.organization_id JOIN roles r ON r.id=ur.role_id AND r.organization_id=ur.organization_id JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1`, c.UserID, org, strings.TrimSpace(code))
	if err != nil {
		return false, nil, err
	}
	defer rows.Close()
	seen := map[int64]bool{}
	for rows.Next() {
		var scope string
		var site, gate sql.NullInt64
		if err := rows.Scan(&scope, &site, &gate); err != nil {
			return false, nil, err
		}
		switch scope {
		case ScopeOrganization:
			return true, nil, nil
		case ScopeSite:
			if site.Valid && site.Int64 > 0 && !seen[site.Int64] {
				seen[site.Int64] = true
				ids = append(ids, site.Int64)
			}
		case ScopeGate:
			if gate.Valid && gate.Int64 > 0 {
				var siteID int64
				e := s.DB.QueryRowContext(ctx, `SELECT g.site_id FROM gates g JOIN sites s ON s.id=g.site_id AND s.organization_id=g.organization_id AND s.is_active=1 WHERE g.organization_id=? AND g.id=? AND g.is_active=1`, org, gate.Int64).Scan(&siteID)
				if e == nil && siteID > 0 && !seen[siteID] {
					seen[siteID] = true
					ids = append(ids, siteID)
				} else if e != nil && e != sql.ErrNoRows {
					return false, nil, e
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return false, nil, err
	}
	if len(ids) == 0 {
		return false, []int64{}, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := []any{org}
	for _, id := range ids {
		args = append(args, id)
	}
	validRows, err := s.DB.QueryContext(ctx, `SELECT id FROM sites WHERE organization_id=? AND is_active=1 AND id IN (`+ph+`)`, args...)
	if err != nil {
		return false, nil, err
	}
	defer validRows.Close()
	valid := make([]int64, 0, len(ids))
	for validRows.Next() {
		var id int64
		if err := validRows.Scan(&id); err != nil {
			return false, nil, err
		}
		valid = append(valid, id)
	}
	return false, valid, validRows.Err()
}

// AccessibleGates expands organization, site and gate grants into the active
// gates the caller may access for a permission.
func (s *Service) AccessibleGates(ctx context.Context, code string) (all bool, ids []int64, err error) {
	c, org, err := s.actor(ctx)
	if err != nil {
		return false, nil, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT ur.scope_type,ur.site_id,ur.gate_id FROM user_roles ur JOIN users u ON u.id=ur.user_id AND u.organization_id=ur.organization_id JOIN roles r ON r.id=ur.role_id AND r.organization_id=ur.organization_id JOIN role_permissions rp ON rp.role_id=ur.role_id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND p.code=? AND ur.is_active=1`, c.UserID, org, strings.TrimSpace(code))
	if err != nil {
		return false, nil, err
	}
	defer rows.Close()
	seen := map[int64]bool{}
	var siteIDs []int64
	for rows.Next() {
		var scope string
		var site, gate sql.NullInt64
		if err := rows.Scan(&scope, &site, &gate); err != nil {
			return false, nil, err
		}
		switch scope {
		case ScopeOrganization:
			return true, nil, nil
		case ScopeSite:
			if site.Valid && site.Int64 > 0 {
				siteIDs = append(siteIDs, site.Int64)
			}
		case ScopeGate:
			if gate.Valid && gate.Int64 > 0 && !seen[gate.Int64] {
				seen[gate.Int64] = true
				ids = append(ids, gate.Int64)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return false, nil, err
	}
	for _, sid := range siteIDs {
		gr, err := s.DB.QueryContext(ctx, `SELECT g.id FROM gates g JOIN sites s ON s.id=g.site_id AND s.organization_id=g.organization_id AND s.is_active=1 WHERE g.organization_id=? AND g.site_id=? AND g.is_active=1`, org, sid)
		if err != nil {
			return false, nil, err
		}
		for gr.Next() {
			var id int64
			if err := gr.Scan(&id); err != nil {
				gr.Close()
				return false, nil, err
			}
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		if err := gr.Err(); err != nil {
			gr.Close()
			return false, nil, err
		}
		gr.Close()
	}
	if len(ids) == 0 {
		return false, []int64{}, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := []any{org}
	for _, id := range ids {
		args = append(args, id)
	}
	validRows, err := s.DB.QueryContext(ctx, `SELECT g.id FROM gates g JOIN sites s ON s.id=g.site_id AND s.organization_id=g.organization_id AND s.is_active=1 WHERE g.organization_id=? AND g.is_active=1 AND g.id IN (`+ph+`)`, args...)
	if err != nil {
		return false, nil, err
	}
	defer validRows.Close()
	valid := make([]int64, 0, len(ids))
	for validRows.Next() {
		var id int64
		if err := validRows.Scan(&id); err != nil {
			return false, nil, err
		}
		valid = append(valid, id)
	}
	return false, valid, validRows.Err()
}
