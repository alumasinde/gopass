package visitors

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)


type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db} }
func (s *Service) List(c context.Context, o int64) ([]Visitor, error) {
	rows, e := s.db.QueryContext(c, `SELECT id,organization_id,first_name,last_name,COALESCE(phone,''),COALESCE(email,''),is_blacklisted FROM visitors WHERE organization_id=? ORDER BY id DESC LIMIT 200`, o)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var x []Visitor
	for rows.Next() {
		var v Visitor
		if e := rows.Scan(&v.ID, &v.OrganizationID, &v.FirstName, &v.LastName, &v.Phone, &v.Email, &v.IsBlacklisted); e != nil {
			return nil, e
		}
		x = append(x, v)
	}
	return x, rows.Err()
}
func (s *Service) Get(c context.Context, o, id int64) (*Visitor, error) {
	var v Visitor
	e := s.db.QueryRowContext(c, `SELECT id,organization_id,first_name,last_name,COALESCE(phone,''),COALESCE(email,''),is_blacklisted FROM visitors WHERE organization_id=? AND id=?`, o, id).Scan(&v.ID, &v.OrganizationID, &v.FirstName, &v.LastName, &v.Phone, &v.Email, &v.IsBlacklisted)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &v, e
}
func (s *Service) Create(c context.Context, o int64, fn, ln, phone, email string) (*Visitor, error) {
	fn = strings.TrimSpace(fn)
	ln = strings.TrimSpace(ln)
	phone = strings.TrimSpace(phone)
	email = strings.ToLower(strings.TrimSpace(email))
	if fn == "" || ln == "" {
		return nil, ErrInvalid
	}
	res, e := s.db.ExecContext(c, `INSERT INTO visitors(organization_id,first_name,last_name,phone,email,is_blacklisted,created_at,updated_at) VALUES(?,?,?,?,?,0,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, o, fn, ln, phone, email)
	if e != nil {
		return nil, e
	}
	id, _ := res.LastInsertId()
	return s.Get(c, o, id)
}
func (s *Service) Update(c context.Context, o, id int64, fn, ln, phone, email string) error {
	if _, e := s.Get(c, o, id); e != nil {
		return e
	}
	fn = strings.TrimSpace(fn)
	ln = strings.TrimSpace(ln)
	if fn == "" || ln == "" {
		return ErrInvalid
	}
	_, e := s.db.ExecContext(c, `UPDATE visitors SET first_name=?,last_name=?,phone=?,email=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, fn, ln, strings.TrimSpace(phone), strings.ToLower(strings.TrimSpace(email)), o, id)
	return e
}
func (s *Service) SetBlacklist(c context.Context, o, id int64, v bool) error {
	if _, e := s.Get(c, o, id); e != nil {
		return e
	}
	_, e := s.db.ExecContext(c, `UPDATE visitors SET is_blacklisted=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, v, o, id)
	return e
}
func (s *Service) Archive(c context.Context, o, id int64) error {
	if _, e := s.Get(c, o, id); e != nil {
		return e
	}
	_, e := s.db.ExecContext(c, `UPDATE visitors SET updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, o, id)
	return e
}
