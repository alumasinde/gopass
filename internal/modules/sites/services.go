package sites

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)


type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db: db} }
func (s *Service) List(ctx context.Context, org int64, limit int) ([]Site, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, e := s.db.QueryContext(ctx, `SELECT id,organization_id,name,code,is_active FROM sites WHERE organization_id=? ORDER BY id DESC LIMIT ?`, org, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Site
	for rows.Next() {
		var v Site
		if e := rows.Scan(&v.ID, &v.OrganizationID, &v.Name, &v.Code, &v.IsActive); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) Get(ctx context.Context, org, id int64) (*Site, error) {
	var v Site
	e := s.db.QueryRowContext(ctx, `SELECT id,organization_id,name,code,is_active FROM sites WHERE organization_id=? AND id=?`, org, id).Scan(&v.ID, &v.OrganizationID, &v.Name, &v.Code, &v.IsActive)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &v, e
}
func (s *Service) Create(ctx context.Context, org int64, name, code string) (*Site, error) {
	name = strings.TrimSpace(name)
	code = strings.TrimSpace(code)
	if name == "" || code == "" {
		return nil, ErrInvalid
	}
	res, e := s.db.ExecContext(ctx, `INSERT INTO sites(organization_id,name,code,is_active,created_at,updated_at) VALUES(?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, org, name, code)
	if e != nil {
		return nil, ErrExists
	}
	id, _ := res.LastInsertId()
	return s.Get(ctx, org, id)
}
func (s *Service) Update(ctx context.Context, org, id int64, name, code string, isActive bool) error {
	if _, e := s.Get(ctx, org, id); e != nil {
		return e
	}
	name = strings.TrimSpace(name)
	code = strings.TrimSpace(code)
	if name == "" || code == "" {
		return ErrInvalid
	}
	_, e := s.db.ExecContext(ctx, `UPDATE sites SET name=?,code=?,is_active=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, name, code, isActive, org, id)
	return e
}
func (s *Service) Delete(ctx context.Context, org, id int64) error {
	if _, e := s.Get(ctx, org, id); e != nil {
		return e
	}
	_, e := s.db.ExecContext(ctx, `UPDATE sites SET is_active=0,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, org, id)
	return e
}
