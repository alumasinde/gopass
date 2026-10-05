package users

import (
	"context"
	"database/sql"
)

// Repository is the persistence boundary for users.
// Tenant-owned operations must receive tenant context before querying MySQL.
type Repository interface {
	Ping(ctx context.Context) error
	Create(ctx context.Context, orgID int64, user *User, passwordHash string) error
	GetByID(ctx context.Context, orgID, id int64) (*User, error)
	GetByEmail(ctx context.Context, orgID int64, email string) (*User, error)
	List(ctx context.Context, orgID int64, limit int) ([]User, error)
	Update(ctx context.Context, orgID int64, user *User) error
	Delete(ctx context.Context, orgID, id int64) error
	GetPasswordHash(ctx context.Context, orgID int64, email string) (string, error)
	UpdateLastLogin(ctx context.Context, orgID, id int64) error
}

type repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) Repository { return &repository{db} }

func (r *repository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

func (r *repository) Create(ctx context.Context, orgID int64, user *User, passwordHash string) error {
	query := `INSERT INTO users(organization_id,first_name,last_name,email,password_hash,is_active,created_at,updated_at) VALUES(?,?,?,?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`
	result, err := r.db.ExecContext(ctx, query, orgID, user.FirstName, user.LastName, user.Email, passwordHash)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	user.ID = id
	user.OrganizationID = orgID
	return nil
}

func (r *repository) GetByID(ctx context.Context, orgID, id int64) (*User, error) {
	var user User
	query := `SELECT id,organization_id,first_name,last_name,email,is_active FROM users WHERE organization_id=? AND id=?`
	err := r.db.QueryRowContext(ctx, query, orgID, id).Scan(&user.ID, &user.OrganizationID, &user.FirstName, &user.LastName, &user.Email, &user.IsActive)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *repository) GetByEmail(ctx context.Context, orgID int64, email string) (*User, error) {
	var user User
	query := `SELECT id,organization_id,first_name,last_name,email,is_active FROM users WHERE organization_id=? AND email=?`
	err := r.db.QueryRowContext(ctx, query, orgID, email).Scan(&user.ID, &user.OrganizationID, &user.FirstName, &user.LastName, &user.Email, &user.IsActive)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *repository) List(ctx context.Context, orgID int64, limit int) ([]User, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	query := `SELECT id,organization_id,first_name,last_name,email,is_active FROM users WHERE organization_id=? ORDER BY id DESC LIMIT ?`
	rows, err := r.db.QueryContext(ctx, query, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.OrganizationID, &user.FirstName, &user.LastName, &user.Email, &user.IsActive); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func (r *repository) Update(ctx context.Context, orgID int64, user *User) error {
	query := `UPDATE users SET first_name=?,last_name=?,email=?,is_active=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`
	_, err := r.db.ExecContext(ctx, query, user.FirstName, user.LastName, user.Email, user.IsActive, orgID, user.ID)
	return err
}

func (r *repository) Delete(ctx context.Context, orgID, id int64) error {
	query := `DELETE FROM users WHERE organization_id=? AND id=?`
	_, err := r.db.ExecContext(ctx, query, orgID, id)
	return err
}

func (r *repository) GetPasswordHash(ctx context.Context, orgID int64, email string) (string, error) {
	var hash string
	query := `SELECT password_hash FROM users WHERE organization_id=? AND email=? AND is_active=1`
	err := r.db.QueryRowContext(ctx, query, orgID, email).Scan(&hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return hash, nil
}

func (r *repository) UpdateLastLogin(ctx context.Context, orgID, id int64) error {
	query := `UPDATE users SET last_login_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`
	_, err := r.db.ExecContext(ctx, query, orgID, id)
	return err
}
