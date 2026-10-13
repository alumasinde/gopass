package identificationtypes

import "time"

type IdentificationType struct {
	ID                int64     `json:"id"`
	OrganizationID    int64     `json:"organization_id"`
	Code              string    `json:"code"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	IsActive          bool      `json:"is_active"`
	IsDefault         bool      `json:"is_default"`
	ValidationRegex   string    `json:"validation_regex"`
	ValidationExample string    `json:"validation_example"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
