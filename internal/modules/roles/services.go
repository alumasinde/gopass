package roles

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrRoleNotFound        = errors.New("role not found")
	ErrRoleAlreadyExists   = errors.New("role already exists")
	ErrInvalidRoleCode     = errors.New("invalid role code")
	ErrInvalidRoleName     = errors.New("invalid role name")
	ErrSystemRoleImmutable = errors.New("system roles cannot be modified")
	ErrInvalidScope        = errors.New("invalid scope type")
)

const (
	ScopeOrganization = "ORGANIZATION"
	ScopeSite         = "SITE"
	ScopeGate         = "GATE"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateRole(ctx context.Context, orgID int64, name, code string) (*Role, error) {
	name = strings.TrimSpace(name)
	code = strings.TrimSpace(code)

	if name == "" {
		return nil, ErrInvalidRoleName
	}
	if code == "" {
		return nil, ErrInvalidRoleCode
	}

	existing, _ := s.repo.GetByCode(ctx, orgID, code)
	if existing != nil {
		return nil, ErrRoleAlreadyExists
	}

	role := &Role{
		Name:     name,
		Code:     code,
		IsSystem: false,
	}

	if err := s.repo.Create(ctx, orgID, role); err != nil {
		return nil, err
	}

	return role, nil
}

func (s *Service) GetRole(ctx context.Context, orgID, id int64) (*Role, error) {
	role, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, ErrRoleNotFound
	}
	return role, nil
}

func (s *Service) ListRoles(ctx context.Context, orgID int64, limit int) ([]Role, error) {
	return s.repo.List(ctx, orgID, limit)
}

func (s *Service) UpdateRole(ctx context.Context, orgID int64, role *Role) error {
	existing, err := s.repo.GetByID(ctx, orgID, role.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrRoleNotFound
	}
	if existing.IsSystem {
		return ErrSystemRoleImmutable
	}

	role.Name = strings.TrimSpace(role.Name)
	role.Code = strings.TrimSpace(role.Code)

	if role.Name == "" {
		return ErrInvalidRoleName
	}
	if role.Code == "" {
		return ErrInvalidRoleCode
	}

	if role.Code != existing.Code {
		codeCheck, _ := s.repo.GetByCode(ctx, orgID, role.Code)
		if codeCheck != nil && codeCheck.ID != role.ID {
			return ErrRoleAlreadyExists
		}
	}

	return s.repo.Update(ctx, orgID, role)
}

func (s *Service) DeleteRole(ctx context.Context, orgID, id int64) error {
	existing, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrRoleNotFound
	}
	if existing.IsSystem {
		return ErrSystemRoleImmutable
	}
	return s.repo.Delete(ctx, orgID, id)
}

func (s *Service) AssignPermission(ctx context.Context, orgID, roleID, permissionID int64) error {
	role, err := s.repo.GetByID(ctx, orgID, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return ErrRoleNotFound
	}
	return s.repo.AssignPermission(ctx, orgID, roleID, permissionID)
}

func (s *Service) RemovePermission(ctx context.Context, orgID, roleID, permissionID int64) error {
	role, err := s.repo.GetByID(ctx, orgID, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return ErrRoleNotFound
	}
	return s.repo.RemovePermission(ctx, orgID, roleID, permissionID)
}

func (s *Service) GetRolePermissions(ctx context.Context, orgID, roleID int64) ([]Permission, error) {
	role, err := s.repo.GetByID(ctx, orgID, roleID)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, ErrRoleNotFound
	}
	return s.repo.GetPermissions(ctx, orgID, roleID)
}

func (s *Service) AssignRoleToUser(ctx context.Context, orgID, userID, roleID int64, scopeType string, siteID, gateID *int64) error {
	role, err := s.repo.GetByID(ctx, orgID, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return ErrRoleNotFound
	}

	if !isValidScope(scopeType) {
		return ErrInvalidScope
	}

	if scopeType == ScopeSite && siteID == nil {
		scopeType = ScopeOrganization
	}
	if scopeType == ScopeGate && gateID == nil {
		scopeType = ScopeOrganization
	}

	return s.repo.AssignToUser(ctx, orgID, userID, roleID, scopeType, siteID, gateID)
}

func (s *Service) RemoveRoleFromUser(ctx context.Context, orgID, userID, roleID int64) error {
	role, err := s.repo.GetByID(ctx, orgID, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return ErrRoleNotFound
	}
	return s.repo.RemoveFromUser(ctx, orgID, userID, roleID)
}

func (s *Service) GetUserRoles(ctx context.Context, orgID, userID int64) ([]UserRole, error) {
	return s.repo.GetUserRoles(ctx, orgID, userID)
}

func isValidScope(scopeType string) bool {
	switch scopeType {
	case ScopeOrganization, ScopeSite, ScopeGate, "":
		return true
	default:
		return false
	}
}
