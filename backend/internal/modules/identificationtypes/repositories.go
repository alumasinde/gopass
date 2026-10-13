package identificationtypes

import (
	"context"
	"database/sql"
	"errors"

	appdatabase "github.com/alumasinde/gopass/internal/app/database"
)

type Repository struct{ DB appdatabase.Handle }

func NewRepository(db any) *Repository { return &Repository{DB: appdatabase.Adapt(db)} }

const selectColumns = `id,organization_id,code,name,COALESCE(description,''),is_active,is_default,COALESCE(validation_regex,''),COALESCE(validation_example,''),created_at,updated_at`

func (r *Repository) EnsureDefaults(ctx context.Context, orgID int64) error {
	defaults := []IdentificationType{
		{Code: "NATIONAL_ID", Name: "National ID", Description: "Government-issued national identification document", IsActive: true, IsDefault: true, ValidationRegex: `^[A-Z0-9]{8,20}$`, ValidationExample: "A123456789"},
		{Code: "PASSPORT", Name: "Passport", Description: "International travel document", IsActive: true, ValidationRegex: `^[A-Z0-9]{6,12}$`, ValidationExample: "AB1234567"},
		{Code: "DRIVERS_LICENSE", Name: "Driving Licence", Description: "Driving licence issued by an authorized authority", IsActive: true, ValidationRegex: `^[A-Z0-9-]{4,30}$`, ValidationExample: "DL-12345678"},
		{Code: "ALIEN_ID", Name: "Alien / Foreign National ID", Description: "Foreign national identification document", IsActive: true, ValidationRegex: `^[A-Z0-9-]{4,30}$`, ValidationExample: "A12345678"},
		{Code: "REFUGEE_ID", Name: "Refugee Identification", Description: "Refugee identification document", IsActive: true, ValidationRegex: `^[A-Z0-9-]{4,30}$`, ValidationExample: "R12345678"},
		{Code: "WORK_PERMIT", Name: "Work Permit", Description: "Work or employment permit", IsActive: true, ValidationRegex: `^[A-Z0-9-]{4,30}$`, ValidationExample: "WP-12345678"},
		{Code: "STUDENT_ID", Name: "Student ID", Description: "School, college or university identification", IsActive: true, ValidationRegex: `^[A-Z0-9-]{3,30}$`, ValidationExample: "STU-12345"},
		{Code: "EMPLOYEE_ID", Name: "Employee ID", Description: "Employer-issued identification", IsActive: true, ValidationRegex: `^[A-Z0-9-]{3,30}$`, ValidationExample: "EMP-12345"},
		{Code: "MILITARY_ID", Name: "Military / Service ID", Description: "Military or authorized service identification", IsActive: true, ValidationRegex: `^[A-Z0-9-]{4,30}$`, ValidationExample: "MIL-12345"},
		{Code: "DIPLOMATIC_ID", Name: "Diplomatic ID", Description: "Diplomatic identification document", IsActive: true, ValidationRegex: `^[A-Z0-9-]{4,30}$`, ValidationExample: "DIP-12345"},
		{Code: "OTHER", Name: "Other Identification", Description: "Other organization-approved identification document", IsActive: true, ValidationRegex: `^[A-Z0-9-]{2,50}$`, ValidationExample: "ID-12345"},
	}
	for _, item := range defaults {
		_, err := r.DB.ExecContext(ctx, `INSERT IGNORE INTO identification_types(organization_id,code,name,description,is_active,is_default,validation_regex,validation_example,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, orgID, item.Code, item.Name, item.Description, item.IsActive, item.IsDefault, item.ValidationRegex, item.ValidationExample)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) List(ctx context.Context, orgID int64) ([]IdentificationType, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT `+selectColumns+` FROM identification_types WHERE organization_id=? ORDER BY is_default DESC,name ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]IdentificationType, 0)
	for rows.Next() {
		var item IdentificationType
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.Code, &item.Name, &item.Description, &item.IsActive, &item.IsDefault, &item.ValidationRegex, &item.ValidationExample, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (r *Repository) Get(ctx context.Context, orgID, id int64) (*IdentificationType, error) {
	var item IdentificationType
	err := r.DB.QueryRowContext(ctx, `SELECT `+selectColumns+` FROM identification_types WHERE organization_id=? AND id=?`, orgID, id).Scan(&item.ID, &item.OrganizationID, &item.Code, &item.Name, &item.Description, &item.IsActive, &item.IsDefault, &item.ValidationRegex, &item.ValidationExample, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &item, err
}
func (r *Repository) GetByCode(ctx context.Context, orgID int64, code string) (*IdentificationType, error) {
	var item IdentificationType
	err := r.DB.QueryRowContext(ctx, `SELECT `+selectColumns+` FROM identification_types WHERE organization_id=? AND code=?`, orgID, code).Scan(&item.ID, &item.OrganizationID, &item.Code, &item.Name, &item.Description, &item.IsActive, &item.IsDefault, &item.ValidationRegex, &item.ValidationExample, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &item, err
}
func (r *Repository) Create(ctx context.Context, item *IdentificationType) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if item.IsDefault {
		if _, err = tx.ExecContext(ctx, `UPDATE identification_types SET is_default=0,updated_at=UTC_TIMESTAMP() WHERE organization_id=?`, item.OrganizationID); err != nil {
			return err
		}
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO identification_types(organization_id,code,name,description,is_active,is_default,validation_regex,validation_example,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, item.OrganizationID, item.Code, item.Name, nullString(item.Description), item.IsActive, item.IsDefault, nullString(item.ValidationRegex), nullString(item.ValidationExample))
	if err != nil {
		return err
	}
	item.ID, err = res.LastInsertId()
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) Update(ctx context.Context, item *IdentificationType) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if item.IsDefault {
		if _, err = tx.ExecContext(ctx, `UPDATE identification_types SET is_default=0,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id<>?`, item.OrganizationID, item.ID); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE identification_types SET name=?,description=?,is_active=?,is_default=?,validation_regex=?,validation_example=?,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=?`, item.Name, nullString(item.Description), item.IsActive, item.IsDefault, nullString(item.ValidationRegex), nullString(item.ValidationExample), item.OrganizationID, item.ID)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) InUse(ctx context.Context, orgID, id int64) (bool, error) {
	var count int
	err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM visitors WHERE organization_id=? AND id_type_id=?`, orgID, id).Scan(&count)
	return count > 0, err
}
func (r *Repository) Delete(ctx context.Context, orgID, id int64) error {
	_, err := r.DB.ExecContext(ctx, `DELETE FROM identification_types WHERE organization_id=? AND id=? AND is_default=0`, orgID, id)
	return err
}
func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
