package passtypes

type PassType struct {
	ID int64 `json:"id"`
	OrganizationID int64 `json:"organization_id"`
	Name string `json:"name"`
	Code string `json:"code"`
	RequiresApproval bool `json:"requires_approval"`
	ValidityMinutes int `json:"validity_minutes"`
	IsActive bool `json:"is_active"`
}
