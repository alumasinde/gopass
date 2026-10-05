package gates

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)



type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db} }
func (s *Service) List(c context.Context, o int64) ([]Gate, error) {
	rows, e := s.db.QueryContext(c, `SELECT id,organization_id,site_id,name,code,is_active FROM gates WHERE organization_id=? ORDER BY id DESC LIMIT 200`, o)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var x []Gate
	for rows.Next() {
		var v Gate
		if e := rows.Scan(&v.ID, &v.OrganizationID, &v.SiteID, &v.Name, &v.Code, &v.IsActive); e != nil {
			return nil, e
		}
		x = append(x, v)
	}
	return x, rows.Err()
}
func (s *Service) Get(c context.Context, o, id int64) (*Gate, error) {
	var v Gate
	e := s.db.QueryRowContext(c, `SELECT id,organization_id,site_id,name,code,is_active FROM gates WHERE organization_id=? AND id=?`, o, id).Scan(&v.ID, &v.OrganizationID, &v.SiteID, &v.Name, &v.Code, &v.IsActive)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &v, e
}
func (s *Service) Create(c context.Context, o, site int64, name, code string) (*Gate, error) {
	name = strings.TrimSpace(name)
	code = strings.TrimSpace(code)
	if site < 1 || name == "" || code == "" {
		return nil, ErrInvalid
	}
	var n int
	if e := s.db.QueryRowContext(c, `SELECT COUNT(*) FROM sites WHERE organization_id=? AND id=? AND is_active=1`, o, site).Scan(&n); e != nil || n == 0 {
		return nil, ErrInvalid
	}
	res, e := s.db.ExecContext(c, `INSERT INTO gates(organization_id,site_id,name,code,is_active,created_at,updated_at) VALUES(?,?,?, ?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, o, site, name, code)
	if e != nil {
		return nil, ErrExists
	}
	id, _ := res.LastInsertId()
	return s.Get(c, o, id)
}
func (s *Service) Update(c context.Context, o, id, site int64, name, code string, active bool) error {
	if _, e := s.Get(c, o, id); e != nil {
		return e
	}
	name = strings.TrimSpace(name)
	code = strings.TrimSpace(code)
	if site < 1 || name == "" || code == "" {
		return ErrInvalid
	}
	var n int
	if e := s.db.QueryRowContext(c, `SELECT COUNT(*) FROM sites WHERE organization_id=? AND id=?`, o, site).Scan(&n); e != nil || n == 0 {
		return ErrInvalid
	}
	_, e := s.db.ExecContext(c, `UPDATE gates SET site_id=?,name=?,code=?,is_active=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, site, name, code, active, o, id)
	return e
}
func (s *Service) Delete(c context.Context, o, id int64) error {
	if _, e := s.Get(c, o, id); e != nil {
		return e
	}
	_, e := s.db.ExecContext(c, `UPDATE gates SET is_active=0,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, o, id)
	return e
}
