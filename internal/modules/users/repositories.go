package users

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Repository is the persistence boundary for users.
// Tenant-owned operations must receive tenant context before querying MySQL.
type Repository interface {
	Ping(ctx context.Context) error
	Create(ctx context.Context, orgID int64, user *User, passwordHash string) error
	GetByID(ctx context.Context, orgID, id int64) (*User, error)
	GetByEmail(ctx context.Context, orgID int64, email string) (*User, error)
	List(ctx context.Context, orgID int64, offset, limit int, search string) ([]User, int64, error)
	Update(ctx context.Context, orgID int64, user *User) error
	Delete(ctx context.Context, orgID, id int64) error
	GetPasswordHash(ctx context.Context, orgID int64, email string) (string, error)
	UpdateLastLogin(ctx context.Context, orgID, id int64) error
	StoreRefreshToken(ctx context.Context, userID, orgID int64, sessionID, tokenHash string, expiresAt time.Time) error
	RotateRefreshToken(ctx context.Context, oldTokenHash string, userID, orgID int64, sessionID, newTokenHash string, expiresAt time.Time) error
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
	RevokeSession(ctx context.Context, sessionID string, userID, orgID int64, tokenHash string) error
	RevokeAllUserRefreshTokens(ctx context.Context, userID int64) error
	IsRefreshTokenValid(ctx context.Context, tokenHash string) (bool, error)
	IsActive(ctx context.Context, userID, orgID int64) (bool, error)
	IsSessionActive(ctx context.Context, sessionID string, userID, orgID int64) (bool, error)
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

func (r *repository) List(ctx context.Context, orgID int64, offset, limit int, search string) ([]User, int64, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	// Build query with optional search
	var query string
	var args []any
	if search != "" {
		searchPattern := "%" + search + "%"
		query = `SELECT id,organization_id,first_name,last_name,email,is_active FROM users WHERE organization_id=? AND (first_name LIKE ? OR last_name LIKE ? OR email LIKE ?) ORDER BY id DESC LIMIT ? OFFSET ?`
		args = []any{orgID, searchPattern, searchPattern, searchPattern, limit, offset}
	} else {
		query = `SELECT id,organization_id,first_name,last_name,email,is_active FROM users WHERE organization_id=? ORDER BY id DESC LIMIT ? OFFSET ?`
		args = []any{orgID, limit, offset}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.OrganizationID, &user.FirstName, &user.LastName, &user.Email, &user.IsActive); err != nil {
			return nil, 0, err
		}
		users = append(users, user)
	}

	// Get total count
	var total int64
	if search != "" {
		searchPattern := "%" + search + "%"
		countQuery := `SELECT COUNT(*) FROM users WHERE organization_id=? AND (first_name LIKE ? OR last_name LIKE ? OR email LIKE ?)`
		err = r.db.QueryRowContext(ctx, countQuery, orgID, searchPattern, searchPattern, searchPattern).Scan(&total)
	} else {
		countQuery := `SELECT COUNT(*) FROM users WHERE organization_id=?`
		err = r.db.QueryRowContext(ctx, countQuery, orgID).Scan(&total)
	}
	if err != nil {
		return nil, 0, err
	}

	return users, total, nil
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

func (r *repository) StoreRefreshToken(ctx context.Context, userID, orgID int64, sessionID, tokenHash string, expiresAt time.Time) error {
	query := `INSERT INTO refresh_tokens(user_id, organization_id, session_id, token_hash, expires_at, created_at) VALUES(?,?,?,?,?,UTC_TIMESTAMP())`
	_, err := r.db.ExecContext(ctx, query, userID, orgID, sessionID, tokenHash, expiresAt)
	return err
}

// RotateRefreshToken atomically consumes one active refresh token and stores its
// replacement. A conditional UPDATE plus RowsAffected check makes concurrent
// refresh attempts single-winner even under READ COMMITTED / REPEATABLE READ.
func (r *repository) RotateRefreshToken(ctx context.Context, oldTokenHash string, userID, orgID int64, sessionID, newTokenHash string, expiresAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at=UTC_TIMESTAMP() WHERE token_hash=? AND user_id=? AND organization_id=? AND session_id=? AND revoked_at IS NULL AND expires_at > UTC_TIMESTAMP()`, oldTokenHash, userID, orgID, sessionID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("refresh token is invalid, expired, or already used")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO refresh_tokens(user_id, organization_id, session_id, token_hash, expires_at, created_at) VALUES(?,?,?,?,?,UTC_TIMESTAMP())`, userID, orgID, sessionID, newTokenHash, expiresAt.UTC()); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *repository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	query := `UPDATE refresh_tokens SET revoked_at=UTC_TIMESTAMP() WHERE token_hash=? AND revoked_at IS NULL`
	_, err := r.db.ExecContext(ctx, query, tokenHash)
	return err
}

func (r *repository) RevokeAllUserRefreshTokens(ctx context.Context, userID int64) error {
	query := `UPDATE refresh_tokens SET revoked_at=UTC_TIMESTAMP() WHERE user_id=? AND revoked_at IS NULL`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

func (r *repository) IsRefreshTokenValid(ctx context.Context, tokenHash string) (bool, error) {
	var revokedAt sql.NullTime
	query := `SELECT revoked_at FROM refresh_tokens WHERE token_hash=? AND expires_at > UTC_TIMESTAMP()`
	err := r.db.QueryRowContext(ctx, query, tokenHash).Scan(&revokedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return !revokedAt.Valid, nil
}

func (r *repository) IsActive(ctx context.Context, userID, orgID int64) (bool, error) {
	var isActive bool
	query := `SELECT u.is_active FROM users u JOIN organizations o ON o.id=u.organization_id AND o.is_active=1 WHERE u.id=? AND u.organization_id=?`
	err := r.db.QueryRowContext(ctx, query, userID, orgID).Scan(&isActive)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return isActive, nil
}

// IsSessionActive checks that the login session still has a live refresh token.
// Revoking the current refresh token therefore revokes its access tokens too.
func (r *repository) IsSessionActive(ctx context.Context, sessionID string, userID, orgID int64) (bool, error) {
	if sessionID == "" {
		return false, nil
	}
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM refresh_tokens WHERE session_id=? AND user_id=? AND organization_id=? AND revoked_at IS NULL AND expires_at > UTC_TIMESTAMP()`, sessionID, userID, orgID).Scan(&n)
	return n > 0, err
}

// RevokeSession revokes every refresh token for a single login session. This
// also invalidates already-issued access tokens because middleware checks sid.
func (r *repository) RevokeSession(ctx context.Context, sessionID string, userID, orgID int64, tokenHash string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at=UTC_TIMESTAMP() WHERE session_id=? AND user_id=? AND organization_id=? AND token_hash=? AND revoked_at IS NULL AND expires_at>UTC_TIMESTAMP()`, sessionID, userID, orgID, tokenHash)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrInvalidRefreshToken
	}
	if _, err = tx.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at=UTC_TIMESTAMP() WHERE session_id=? AND user_id=? AND organization_id=? AND revoked_at IS NULL`, sessionID, userID, orgID); err != nil {
		return err
	}
	return tx.Commit()
}
