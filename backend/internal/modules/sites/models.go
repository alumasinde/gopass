package sites

type Site struct {
	ID             int64  `json:"id"`
	OrganizationID int64  `json:"organization_id"`
	Name           string `json:"name"`
	Code           string `json:"code"`
	IsActive       bool   `json:"is_active"`
}
