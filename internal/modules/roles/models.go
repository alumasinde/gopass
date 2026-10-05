package roles

type Role struct {
	ID int64 `json:"id"`
	OrganizationID int64 `json:"organization_id"`
	Name string `json:"name"`
	Code string `json:"code"`
	IsSystem bool `json:"is_system"`
}
