package users

import "time"

type User struct {
	ID             int64      `json:"id"`
	OrganizationID int64      `json:"organization_id"`
	FirstName      string     `json:"first_name"`
	LastName       string     `json:"last_name"`
	Email          string     `json:"email"`
	PasswordHash   string     `json:"-"`
	IsActive       bool       `json:"is_active"`
	LastLoginAt    *time.Time `json:"last_login_at,omitempty"`
}
