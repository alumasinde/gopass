package users

type User struct {
	ID int64 `json:"id"`
	OrganizationID int64 `json:"organization_id"`
	FirstName string `json:"first_name"`
	LastName string `json:"last_name"`
	Email string `json:"email"`
	IsActive bool `json:"is_active"`
}
