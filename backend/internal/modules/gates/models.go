package gates

type Gate struct {
	ID             int64  `json:"id"`
	OrganizationID int64  `json:"organization_id"`
	SiteID         int64  `json:"site_id"`
	Name           string `json:"name"`
	Code           string `json:"code"`
	IsActive       bool   `json:"is_active"`
}
