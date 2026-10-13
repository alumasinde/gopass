package identificationtypes

import (
	"context"
	"database/sql"

	appdatabase "github.com/alumasinde/gopass/internal/app/database"
)

type Repository struct {
	DB appdatabase.Handle
}

func NewRepository(db any) *Repository {
	return &Repository{DB: appdatabase.Adapt(db)}
}

func (r *Repository) List(ctx context.Context, orgID int64) ([]IdentificationType, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT id, organization_id, code, name, description, is_active, is_default, validation_regex, validation_example, created_at, updated_at
		FROM identification_types
		WHERE organization_id = ?
		ORDER BY is_default DESC, name ASC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []IdentificationType
	for rows.Next() {
		var item IdentificationType
		if err := rows.Scan(
			&item.ID, &item.OrganizationID, &item.Code, &item.Name, &item.Description,
			&item.IsActive, &item.IsDefault, &item.ValidationRegex, &item.ValidationExample,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Get(ctx context.Context, orgID, id int64) (*IdentificationType, error) {
	var item IdentificationType
	err := r.DB.QueryRowContext(ctx, `
		SELECT id, organization_id, code, name, description, is_active, is_default, validation_regex, validation_example, created_at, updated_at
		FROM identification_types
		WHERE organization_id = ? AND id = ?
	`, orgID, id).Scan(
		&item.ID, &item.OrganizationID, &item.Code, &item.Name, &item.Description,
		&item.IsActive, &item.IsDefault, &item.ValidationRegex, &item.ValidationExample,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &item, err
}

func (r *Repository) GetByCode(ctx context.Context, orgID int64, code string) (*IdentificationType, error) {
	var item IdentificationType
	err := r.DB.QueryRowContext(ctx, `
		SELECT id, organization_id, code, name, description, is_active, is_default, validation_regex, validation_example, created_at, updated_at
		FROM identification_types
		WHERE organization_id = ? AND code = ?
	`, orgID, code).Scan(
		&item.ID, &item.OrganizationID, &item.Code, &item.Name, &item.Description,
		&item.IsActive, &item.IsDefault, &item.ValidationRegex, &item.ValidationExample,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &item, err
}

func (r *Repository) Create(ctx context.Context, item *IdentificationType) error {
	result, err := r.DB.ExecContext(ctx, `
		INSERT INTO identification_types (organization_id, code, name, description, is_active, is_default, validation_regex, validation_example, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, UTC_TIMESTAMP(), UTC_TIMESTAMP())
	`, item.OrganizationID, item.Code, item.Name, item.Description, item.IsActive, item.IsDefault, item.ValidationRegex, item.ValidationExample)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	item.ID = id
	return nil
}

func (r *Repository) Update(ctx context.Context, item *IdentificationType) error {
	_, err := r.DB.ExecContext(ctx, `
		UPDATE identification_types
		SET name = ?, description = ?, is_active = ?, is_default = ?, validation_regex = ?, validation_example = ?, updated_at = UTC_TIMESTAMP()
		WHERE organization_id = ? AND id = ?
	`, item.Name, item.Description, item.IsActive, item.IsDefault, item.ValidationRegex, item.ValidationExample, item.OrganizationID, item.ID)
	return err
}

func (r *Repository) Delete(ctx context.Context, orgID, id int64) error {
	_, err := r.DB.ExecContext(ctx, `
		DELETE FROM identification_types WHERE organization_id = ? AND id = ? AND is_default = FALSE
	`, orgID, id)
	return err
}
