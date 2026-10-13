package visits

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/alumasinde/gopass/internal/app/database"
)

type Service struct{ db database.Handle }

func NewService(db any) *Service { return &Service{database.Adapt(db)} }

const visitSelect = `
	SELECT v.id,v.organization_id,v.visitor_id,v.site_id,v.gate_id,v.direction,v.visit_date,v.check_in_time,v.check_out_time,
	       COALESCE(v.purpose,''),COALESCE(v.host_name,''),COALESCE(v.host_department,''),
	       v.is_blacklisted,v.blacklist_reason,COALESCE(v.notes,''),v.created_at,v.updated_at
	FROM visits v
`

func (s *Service) List(c context.Context, orgID int64) ([]Visit, error) {
	rows, err := s.db.QueryContext(c, visitSelect+` WHERE v.organization_id=? ORDER BY v.id DESC LIMIT 200`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Visit, 0)
	for rows.Next() {
		item, err := scanVisit(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanVisit(row scanner) (Visit, error) {
	var v Visit
	var checkOut sql.NullString
	var blacklistReason sql.NullString
	err := row.Scan(&v.ID, &v.OrganizationID, &v.VisitorID, &v.SiteID, &v.GateID, &v.Direction,
		&v.VisitDate, &v.CheckInTime, &checkOut, &v.Purpose, &v.HostName, &v.HostDepartment,
		&v.IsBlacklisted, &blacklistReason, &v.Notes, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return v, err
	}
	if checkOut.Valid {
		v.CheckOutTime = &checkOut.String
	}
	if blacklistReason.Valid {
		v.BlacklistReason = &blacklistReason.String
	}
	return v, nil
}

func (s *Service) Get(c context.Context, orgID, id int64) (*Visit, error) {
	v, err := scanVisit(s.db.QueryRowContext(c, visitSelect+` WHERE v.organization_id=? AND v.id=?`, orgID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *Service) Create(c context.Context, orgID int64, in CreateInput) (*Visit, error) {
	if in.VisitorID < 1 || in.SiteID < 1 || in.GateID < 1 {
		return nil, ErrInvalid
	}
	if in.Direction != "IN" && in.Direction != "OUT" {
		return nil, ErrInvalid
	}
	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Lock and validate visitor
	var blacklisted bool
	var blacklistReason *string
	err = tx.QueryRowContext(c, `SELECT (v.is_blacklisted OR EXISTS (
		SELECT 1 FROM blacklist b WHERE b.organization_id=v.organization_id AND b.is_active=1
		AND (b.expires_at IS NULL OR b.expires_at>UTC_TIMESTAMP())
		AND (b.visitor_id=v.id OR (NULLIF(TRIM(b.id_number),'') IS NOT NULL AND UPPER(TRIM(b.id_number))=UPPER(TRIM(COALESCE(v.id_number,''))) AND (b.id_type_id IS NULL OR b.id_type_id=v.id_type_id)))
	), b.reason FROM visitors v LEFT JOIN blacklist b ON b.visitor_id=v.id AND b.is_active=1 AND b.organization_id=v.organization_id WHERE v.organization_id=? AND v.id=? AND v.archived_at IS NULL FOR UPDATE`, orgID, in.VisitorID).Scan(&blacklisted, &blacklistReason)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	if blacklisted {
		return nil, ErrBlacklisted
	}

	// Validate site and gate
	var siteActive, gateActive bool
	err = tx.QueryRowContext(c, `SELECT is_active FROM sites WHERE organization_id=? AND id=? FOR UPDATE`, orgID, in.SiteID).Scan(&siteActive)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !siteActive) {
		return nil, ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	err = tx.QueryRowContext(c, `SELECT is_active FROM gates WHERE organization_id=? AND id=? FOR UPDATE`, orgID, in.GateID).Scan(&gateActive)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !gateActive) {
		return nil, ErrInvalid
	}
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	visitDate := now.Format("2006-01-02")
	checkInTime := now.Format("2006-01-02 15:04:05")

	var blacklistReasonValue any
	if blacklistReason != nil {
		blacklistReasonValue = *blacklistReason
	}

	res, err := tx.ExecContext(c, `INSERT INTO visits(organization_id,visitor_id,site_id,gate_id,direction,visit_date,check_in_time,purpose,host_name,host_department,is_blacklisted,blacklist_reason,notes,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`,
		orgID, in.VisitorID, in.SiteID, in.GateID, in.Direction, visitDate, checkInTime,
		nullString(in.Purpose), nullString(in.HostName), nullString(in.HostDepartment),
		false, blacklistReasonValue, nullString(in.Notes))
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return s.Get(c, orgID, id)
}

func (s *Service) Update(c context.Context, orgID, id int64, in UpdateInput) error {
	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRowContext(c, `SELECT COUNT(*) FROM visits WHERE organization_id=? AND id=?`, orgID, id).Scan(&exists)
	if err != nil || exists == 0 {
		return ErrNotFound
	}

	var currentCheckOut sql.NullString
	err = tx.QueryRowContext(c, `SELECT check_out_time FROM visits WHERE organization_id=? AND id=? FOR UPDATE`, orgID, id).Scan(&currentCheckOut)
	if err != nil {
		return err
	}

	// If checking out, validate it's not already checked out
	if in.CheckOutTime != nil && currentCheckOut.Valid {
		return ErrAlreadyClosed
	}

	updates := []string{}
	args := []any{orgID, id}
	argCount := 2

	if in.CheckOutTime != nil {
		updates = append(updates, "check_out_time=?")
		args = append(args, *in.CheckOutTime)
		argCount++
	}
	if in.Purpose != "" {
		updates = append(updates, "purpose=?")
		args = append(args, in.Purpose)
		argCount++
	}
	if in.HostName != "" {
		updates = append(updates, "host_name=?")
		args = append(args, in.HostName)
		argCount++
	}
	if in.HostDepartment != "" {
		updates = append(updates, "host_department=?")
		args = append(args, in.HostDepartment)
		argCount++
	}
	if in.Notes != "" {
		updates = append(updates, "notes=?")
		args = append(args, in.Notes)
		argCount++
	}

	if len(updates) == 0 {
		return nil
	}

	updates = append(updates, "updated_at=UTC_TIMESTAMP()")
	query := "UPDATE visits SET " + strings.Join(updates, ",") + " WHERE organization_id=? AND id=?"
	_, err = tx.ExecContext(c, query, args...)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) Delete(c context.Context, orgID, id int64) error {
	res, err := s.db.ExecContext(c, `DELETE FROM visits WHERE organization_id=? AND id=?`, orgID, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
