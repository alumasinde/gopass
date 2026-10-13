package passtypes

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db: db} }

func (s *Service) List(ctx context.Context, org int64) ([]PassType, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,organization_id,name,code,requires_approval,validity_minutes,is_active FROM pass_types WHERE organization_id=? ORDER BY name`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PassType
	for rows.Next() {
		var v PassType
		if err := rows.Scan(&v.ID, &v.OrganizationID, &v.Name, &v.Code, &v.RequiresApproval, &v.ValidityMinutes, &v.IsActive); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) Get(ctx context.Context, org, id int64) (*PassType, error) {
	var v PassType
	err := s.db.QueryRowContext(ctx, `SELECT id,organization_id,name,code,requires_approval,validity_minutes,is_active FROM pass_types WHERE organization_id=? AND id=?`, org, id).Scan(&v.ID, &v.OrganizationID, &v.Name, &v.Code, &v.RequiresApproval, &v.ValidityMinutes, &v.IsActive)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *Service) Create(ctx context.Context, org int64, name, code string, requires bool, validity int) (*PassType, error) {
	name = strings.TrimSpace(name)
	code = strings.TrimSpace(code)
	if name == "" || code == "" || validity < 1 || validity > 10080 {
		return nil, ErrInvalid
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO pass_types(organization_id,name,code,requires_approval,validity_minutes,is_active,created_at,updated_at) VALUES(?,?,?,?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, org, name, code, requires, validity)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, org, id)
}

func (s *Service) Update(ctx context.Context, org, id int64, name, code string, requires bool, validity int, isActive bool) error {
	if _, err := s.Get(ctx, org, id); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	code = strings.TrimSpace(code)
	if name == "" || code == "" || validity < 1 || validity > 10080 {
		return ErrInvalid
	}
	_, err := s.db.ExecContext(ctx, `UPDATE pass_types SET name=?,code=?,requires_approval=?,validity_minutes=?,is_active=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, name, code, requires, validity, isActive, org, id)
	return err
}

func (s *Service) Delete(ctx context.Context, org, id int64) error {
	if _, err := s.Get(ctx, org, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE pass_types SET is_active=0,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, org, id)
	return err
}
