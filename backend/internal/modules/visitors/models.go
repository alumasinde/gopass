package visitors

type Visitor struct {
	ID             int64  `json:"id"`
	OrganizationID int64  `json:"organization_id"`
	FirstName      string `json:"first_name"`
	LastName       string `json:"last_name"`
	Phone          string `json:"phone"`
	Email          string `json:"email"`
	IsBlacklisted  bool   `json:"is_blacklisted"`
}
