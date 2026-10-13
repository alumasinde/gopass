package visitors

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/alumasinde/gopass/internal/app/database"
)

type Service struct{ db database.Handle }

func NewService(db any) *Service { return &Service{database.Adapt(db)} }

const visitorSelect = `
	SELECT v.id,v.organization_id,v.first_name,v.last_name,COALESCE(v.company_name,''),COALESCE(v.phone,''),COALESCE(v.email,''),
	       v.id_type_id,COALESCE(it.code,''),COALESCE(it.name,''),COALESCE(v.id_number,''),v.id_expiry_date,
	       (v.is_blacklisted OR EXISTS (
	          SELECT 1 FROM blacklist b
	          WHERE b.organization_id=v.organization_id AND b.is_active=1
	            AND (b.expires_at IS NULL OR b.expires_at>UTC_TIMESTAMP())
	            AND ((b.visitor_id=v.id) OR (NULLIF(TRIM(b.id_number),'') IS NOT NULL AND UPPER(TRIM(b.id_number))=UPPER(TRIM(COALESCE(v.id_number,''))) AND (b.id_type_id IS NULL OR b.id_type_id=v.id_type_id)))
	       )) AS effectively_blacklisted
	FROM visitors v
	LEFT JOIN identification_types it ON it.id=v.id_type_id AND it.organization_id=v.organization_id
`

func (s *Service) List(c context.Context, orgID int64) ([]Visitor, error) {
	rows, err := s.db.QueryContext(c, visitorSelect+` WHERE v.organization_id=? AND v.archived_at IS NULL ORDER BY v.id DESC LIMIT 200`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Visitor, 0)
	for rows.Next() {
		item, err := scanVisitor(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanVisitor(row scanner) (Visitor, error) {
	var v Visitor
	var idType sql.NullInt64
	var expiry sql.NullTime
	err := row.Scan(&v.ID, &v.OrganizationID, &v.FirstName, &v.LastName, &v.CompanyName, &v.Phone, &v.Email,
		&idType, &v.IDTypeCode, &v.IDTypeName, &v.IDNumber, &expiry, &v.IsBlacklisted)
	if err != nil {
		return v, err
	}
	if idType.Valid {
		value := idType.Int64
		v.IDTypeID = &value
	}
	if expiry.Valid {
		v.IDExpiryDate = expiry.Time.Format("2006-01-02")
	}
	v.RiskScore, v.RiskLevel, v.RiskFactors = calculateRisk(v, time.Now().UTC())
	return v, nil
}

func (s *Service) Get(c context.Context, orgID, id int64) (*Visitor, error) {
	v, err := scanVisitor(s.db.QueryRowContext(c, visitorSelect+` WHERE v.organization_id=? AND v.id=? AND v.archived_at IS NULL`, orgID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func calculateRisk(v Visitor, now time.Time) (int, string, []string) {
	score := 0
	factors := make([]string, 0, 5)
	if strings.TrimSpace(v.Phone) == "" {
		score += 20
		factors = append(factors, "No phone number supplied (+20)")
	}
	if strings.TrimSpace(v.Email) == "" {
		score += 15
		factors = append(factors, "No email address supplied (+15)")
	}
	if v.IDTypeID == nil || strings.TrimSpace(v.IDNumber) == "" {
		score += 30
		factors = append(factors, "Identification type or number missing (+30)")
	}
	if v.IDExpiryDate != "" {
		if expiry, err := time.Parse("2006-01-02", v.IDExpiryDate); err == nil && expiry.Before(now.Truncate(24*time.Hour)) {
			score += 15
			factors = append(factors, "Identification document expired (+15)")
		}
	}
	if strings.TrimSpace(v.CompanyName) == "" {
		score += 5
		factors = append(factors, "Company or affiliation not supplied (+5)")
	}
	if v.IsBlacklisted {
		return 100, "CRITICAL", append(factors, "Matches an active blacklist record")
	}
	if score > 100 {
		score = 100
	}
	level := "LOW"
	switch {
	case score >= 75:
		level = "CRITICAL"
	case score >= 50:
		level = "HIGH"
	case score >= 25:
		level = "MEDIUM"
	}
	if len(factors) == 0 {
		factors = append(factors, "No configured risk factors detected")
	}
	return score, level, factors
}

func (s *Service) validateIdentity(ctx context.Context, orgID int64, typeID int64, number, expiry string) error {
	number = strings.ToUpper(strings.TrimSpace(number))
	if typeID == 0 && number == "" && strings.TrimSpace(expiry) == "" {
		return nil
	}
	if typeID < 1 || number == "" {
		return fmt.Errorf("%w: identification type and number must be supplied together", ErrInvalid)
	}
	var active bool
	var pattern string
	err := s.db.QueryRowContext(ctx, `SELECT is_active,COALESCE(validation_regex,'') FROM identification_types WHERE organization_id=? AND id=?`, orgID, typeID).Scan(&active, &pattern)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !active) {
		return fmt.Errorf("%w: identification type is unavailable", ErrInvalid)
	}
	if err != nil {
		return err
	}
	if pattern != "" {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("%w: configured identification validation is invalid", ErrInvalid)
		}
		if !re.MatchString(number) {
			return fmt.Errorf("%w: identification number does not match the selected type", ErrInvalid)
		}
	}
	if expiry != "" {
		if _, err := time.Parse("2006-01-02", expiry); err != nil {
			return fmt.Errorf("%w: identification expiry must use YYYY-MM-DD", ErrInvalid)
		}
	}
	return nil
}

func (s *Service) Create(c context.Context, orgID int64, in Input) (*Visitor, error) {
	in.normalize()
	if in.FirstName == "" || in.LastName == "" {
		return nil, ErrInvalid
	}
	if err := s.validateIdentity(c, orgID, in.IDTypeID, in.IDNumber, in.IDExpiryDate); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// Serialize identity uniqueness checks for this organization so concurrent
	// registrations cannot race past the duplicate check.
	var lockedOrg int64
	if err = tx.QueryRowContext(c, `SELECT id FROM organizations WHERE id=? FOR UPDATE`, orgID).Scan(&lockedOrg); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalid
		}
		return nil, err
	}
	if in.IDNumber != "" {
		var count int
		err = tx.QueryRowContext(c, `SELECT COUNT(*) FROM visitors WHERE organization_id=? AND id_type_id=? AND UPPER(TRIM(id_number))=? AND archived_at IS NULL`, orgID, in.IDTypeID, in.IDNumber).Scan(&count)
		if err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, ErrDuplicateIdentity
		}
	}
	var typeValue any
	if in.IDTypeID > 0 {
		typeValue = in.IDTypeID
	}
	var expiryValue any
	if in.IDExpiryDate != "" {
		expiryValue = in.IDExpiryDate
	}
	res, err := tx.ExecContext(c, `INSERT INTO visitors(organization_id,first_name,last_name,company_name,phone,email,id_type_id,id_number,id_expiry_date,is_blacklisted,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,0,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, orgID, in.FirstName, in.LastName, nullString(in.CompanyName), nullString(in.Phone), nullString(in.Email), typeValue, nullString(in.IDNumber), expiryValue)
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

func (s *Service) Update(c context.Context, orgID, id int64, in Input) error {
	in.normalize()
	if in.FirstName == "" || in.LastName == "" {
		return ErrInvalid
	}
	if err := s.validateIdentity(c, orgID, in.IDTypeID, in.IDNumber, in.IDExpiryDate); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var lockedOrg int64
	if err = tx.QueryRowContext(c, `SELECT id FROM organizations WHERE id=? FOR UPDATE`, orgID).Scan(&lockedOrg); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	var exists int
	if err = tx.QueryRowContext(c, `SELECT COUNT(*) FROM visitors WHERE organization_id=? AND id=? AND archived_at IS NULL`, orgID, id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrNotFound
	}
	if in.IDNumber != "" {
		var count int
		err = tx.QueryRowContext(c, `SELECT COUNT(*) FROM visitors WHERE organization_id=? AND id_type_id=? AND UPPER(TRIM(id_number))=? AND id<>? AND archived_at IS NULL`, orgID, in.IDTypeID, in.IDNumber, id).Scan(&count)
		if err != nil {
			return err
		}
		if count > 0 {
			return ErrDuplicateIdentity
		}
	}
	var typeValue any
	if in.IDTypeID > 0 {
		typeValue = in.IDTypeID
	}
	var expiryValue any
	if in.IDExpiryDate != "" {
		expiryValue = in.IDExpiryDate
	}
	_, err = tx.ExecContext(c, `UPDATE visitors SET first_name=?,last_name=?,company_name=?,phone=?,email=?,id_type_id=?,id_number=?,id_expiry_date=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND archived_at IS NULL`,
		in.FirstName, in.LastName, nullString(in.CompanyName), nullString(in.Phone), nullString(in.Email), typeValue, nullString(in.IDNumber), expiryValue, orgID, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) SetBlacklist(c context.Context, orgID, id int64, blocked bool, actorID int64) error {
	v, err := s.Get(c, orgID, id)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if blocked {
		if _, err = tx.ExecContext(c, `UPDATE visitors SET is_blacklisted=1,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, orgID, id); err != nil {
			return err
		}
		var typeValue any
		if v.IDTypeID != nil {
			typeValue = *v.IDTypeID
		}
		var idValue any
		if v.IDNumber != "" {
			idValue = v.IDNumber
		}
		name := strings.TrimSpace(v.FirstName + " " + v.LastName)
		_, err = tx.ExecContext(c, `INSERT INTO blacklist(organization_id,visitor_id,id_type_id,id_number,name,reason,severity,is_active,added_by,created_at,updated_at)
			VALUES(?,?,?,?,?,'Manually blacklisted from visitor profile','HIGH',1,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, orgID, id, typeValue, idValue, name, actorID)
		if err != nil {
			return err
		}
	} else {
		if _, err = tx.ExecContext(c, `UPDATE visitors SET is_blacklisted=0,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, orgID, id); err != nil {
			return err
		}
		if _, err = tx.ExecContext(c, `UPDATE blacklist SET is_active=0,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND visitor_id=? AND is_active=1`, orgID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) Archive(c context.Context, orgID, id int64) error {
	if _, err := s.Get(c, orgID, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(c, `UPDATE visitors SET archived_at=UTC_TIMESTAMP(),updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND archived_at IS NULL`, orgID, id)
	return err
}

func nullString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return strings.TrimSpace(v)
}
