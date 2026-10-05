package organizations

import (
	"context"
	"database/sql"
)

// Repository is the persistence boundary for organizations.
// Tenant-owned operations must receive tenant context before querying MySQL.
type Repository interface {
	Ping(ctx context.Context) error
	Create(ctx context.Context, org *Organization) error
	GetByID(ctx context.Context, id int64) (*Organization, error)
	GetBySlug(ctx context.Context, slug string) (*Organization, error)
	List(ctx context.Context, limit int) ([]Organization, error)
	Update(ctx context.Context, org *Organization) error
	Delete(ctx context.Context, id int64) error
}

type repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) Repository { return &repository{db} }

func (r *repository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

func (r *repository) Create(ctx context.Context, org *Organization) error {
	query := `INSERT INTO organizations(name,slug,is_active,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`
	result, err := r.db.ExecContext(ctx, query, org.Name, org.Slug, org.IsActive)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	org.ID = id
	return nil
}

func (r *repository) GetByID(ctx context.Context, id int64) (*Organization, error) {
	var org Organization
	query := `SELECT id,name,slug,is_active FROM organizations WHERE id=?`
	err := r.db.QueryRowContext(ctx, query, id).Scan(&org.ID, &org.Name, &org.Slug, &org.IsActive)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &org, nil
}

func (r *repository) GetBySlug(ctx context.Context, slug string) (*Organization, error) {
	var org Organization
	query := `SELECT id,name,slug,is_active FROM organizations WHERE slug=?`
	err := r.db.QueryRowContext(ctx, query, slug).Scan(&org.ID, &org.Name, &org.Slug, &org.IsActive)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &org, nil
}

func (r *repository) List(ctx context.Context, limit int) ([]Organization, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	query := `SELECT id,name,slug,is_active FROM organizations ORDER BY id DESC LIMIT ?`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var orgs []Organization
	for rows.Next() {
		var org Organization
		if err := rows.Scan(&org.ID, &org.Name, &org.Slug, &org.IsActive); err != nil {
			return nil, err
		}
		orgs = append(orgs, org)
	}
	return orgs, nil
}

func (r *repository) Update(ctx context.Context, org *Organization) error {
	query := `UPDATE organizations SET name=?,slug=?,is_active=?,updated_at=UTC_TIMESTAMP() WHERE id=?`
	_, err := r.db.ExecContext(ctx, query, org.Name, org.Slug, org.IsActive, org.ID)
	return err
}

func (r *repository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM organizations WHERE id=?`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}
