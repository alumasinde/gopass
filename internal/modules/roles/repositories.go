package roles

import (
	"context"
	"database/sql"
)

// Repository is the persistence boundary for roles.
// Tenant-owned operations must receive tenant context before querying MySQL.
type Repository interface {
	Ping(ctx context.Context) error
	Create(ctx context.Context, orgID int64, role *Role) error
	GetByID(ctx context.Context, orgID, id int64) (*Role, error)
	GetByCode(ctx context.Context, orgID int64, code string) (*Role, error)
	List(ctx context.Context, orgID int64, limit int) ([]Role, error)
	Update(ctx context.Context, orgID int64, role *Role) error
	Delete(ctx context.Context, orgID, id int64) error
	AssignPermission(ctx context.Context, orgID, roleID, permissionID int64) error
	RemovePermission(ctx context.Context, orgID, roleID, permissionID int64) error
	GetPermissions(ctx context.Context, orgID, roleID int64) ([]Permission, error)
	AssignToUser(ctx context.Context, orgID, userID, roleID int64, scopeType string, siteID, gateID *int64) error
	RemoveFromUser(ctx context.Context, orgID, userID, roleID int64) error
	GetUserRoles(ctx context.Context, orgID, userID int64) ([]UserRole, error)
}

type repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) Repository { return &repository{db} }

func (r *repository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

func (r *repository) Create(ctx context.Context, orgID int64, role *Role) error {
	query := `INSERT INTO roles(organization_id,name,code,is_system,created_at,updated_at) VALUES(?,?,?,0,UTC_TIMESTAMP(),UTC_TIMESTAMP())`
	result, err := r.db.ExecContext(ctx, query, orgID, role.Name, role.Code)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	role.ID = id
	role.OrganizationID = orgID
	return nil
}

func (r *repository) GetByID(ctx context.Context, orgID, id int64) (*Role, error) {
	var role Role
	query := `SELECT id,organization_id,name,code,is_system FROM roles WHERE organization_id=? AND id=?`
	err := r.db.QueryRowContext(ctx, query, orgID, id).Scan(&role.ID, &role.OrganizationID, &role.Name, &role.Code, &role.IsSystem)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

func (r *repository) GetByCode(ctx context.Context, orgID int64, code string) (*Role, error) {
	var role Role
	query := `SELECT id,organization_id,name,code,is_system FROM roles WHERE organization_id=? AND code=?`
	err := r.db.QueryRowContext(ctx, query, orgID, code).Scan(&role.ID, &role.OrganizationID, &role.Name, &role.Code, &role.IsSystem)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

func (r *repository) List(ctx context.Context, orgID int64, limit int) ([]Role, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	query := `SELECT id,organization_id,name,code,is_system FROM roles WHERE organization_id=? ORDER BY name LIMIT ?`
	rows, err := r.db.QueryContext(ctx, query, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.OrganizationID, &role.Name, &role.Code, &role.IsSystem); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, nil
}

func (r *repository) Update(ctx context.Context, orgID int64, role *Role) error {
	query := `UPDATE roles SET name=?,code=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND is_system=0`
	_, err := r.db.ExecContext(ctx, query, role.Name, role.Code, orgID, role.ID)
	return err
}

func (r *repository) Delete(ctx context.Context, orgID, id int64) error {
	query := `DELETE FROM roles WHERE organization_id=? AND id=? AND is_system=0`
	_, err := r.db.ExecContext(ctx, query, orgID, id)
	return err
}

func (r *repository) AssignPermission(ctx context.Context, orgID, roleID, permissionID int64) error {
	query := `INSERT IGNORE INTO role_permissions(role_id,permission_id) SELECT ?,id FROM permissions WHERE id=? AND EXISTS(SELECT 1 FROM roles WHERE id=? AND organization_id=?)`
	_, err := r.db.ExecContext(ctx, query, roleID, permissionID, roleID, orgID)
	return err
}

func (r *repository) RemovePermission(ctx context.Context, orgID, roleID, permissionID int64) error {
	query := `DELETE FROM role_permissions WHERE role_id=? AND permission_id=? AND EXISTS(SELECT 1 FROM roles WHERE id=? AND organization_id=?)`
	_, err := r.db.ExecContext(ctx, query, roleID, permissionID, roleID, orgID)
	return err
}

func (r *repository) GetPermissions(ctx context.Context, orgID, roleID int64) ([]Permission, error) {
	query := `SELECT p.id,p.code,p.name,p.description,p.module,p.action,p.is_system FROM permissions p JOIN role_permissions rp ON rp.permission_id=p.id WHERE rp.role_id=? AND EXISTS(SELECT 1 FROM roles WHERE id=? AND organization_id=?)`
	rows, err := r.db.QueryContext(ctx, query, roleID, roleID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var perms []Permission
	for rows.Next() {
		var perm Permission
		if err := rows.Scan(&perm.ID, &perm.Code, &perm.Name, &perm.Description, &perm.Module, &perm.Action, &perm.IsSystem); err != nil {
			return nil, err
		}
		perms = append(perms, perm)
	}
	return perms, nil
}

func (r *repository) AssignToUser(ctx context.Context, orgID, userID, roleID int64, scopeType string, siteID, gateID *int64) error {
	query := `INSERT INTO user_roles(user_id,role_id,organization_id,scope_type,site_id,gate_id,is_active,created_at,updated_at) SELECT ?,id,?,COALESCE(NULLIF(?,''),'ORGANIZATION'),?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP() FROM roles WHERE id=? AND organization_id=?`
	_, err := r.db.ExecContext(ctx, query, userID, orgID, scopeType, siteID, gateID, roleID, orgID)
	return err
}

func (r *repository) RemoveFromUser(ctx context.Context, orgID, userID, roleID int64) error {
	query := `DELETE FROM user_roles WHERE user_id=? AND role_id=? AND organization_id=?`
	_, err := r.db.ExecContext(ctx, query, userID, roleID, orgID)
	return err
}

func (r *repository) GetUserRoles(ctx context.Context, orgID, userID int64) ([]UserRole, error) {
	query := `SELECT ur.id,ur.user_id,ur.role_id,ur.organization_id,ur.scope_type,ur.site_id,ur.gate_id,ur.is_active,r.name,r.code FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=? AND ur.organization_id=? AND ur.is_active=1`
	rows, err := r.db.QueryContext(ctx, query, userID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var userRoles []UserRole
	for rows.Next() {
		var ur UserRole
		if err := rows.Scan(&ur.ID, &ur.UserID, &ur.RoleID, &ur.OrganizationID, &ur.ScopeType, &ur.SiteID, &ur.GateID, &ur.IsActive, &ur.RoleName, &ur.RoleCode); err != nil {
			return nil, err
		}
		userRoles = append(userRoles, ur)
	}
	return userRoles, nil
}
