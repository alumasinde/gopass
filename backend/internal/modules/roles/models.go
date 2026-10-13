package roles

type Role struct {
	ID             int64  `json:"id"`
	OrganizationID int64  `json:"organization_id"`
	Name           string `json:"name"`
	Code           string `json:"code"`
	IsSystem       bool   `json:"is_system"`
}

type Permission struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Module      string `json:"module"`
	Action      string `json:"action"`
	IsSystem    bool   `json:"is_system"`
}

type UserRole struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"user_id"`
	RoleID         int64  `json:"role_id"`
	OrganizationID int64  `json:"organization_id"`
	ScopeType      string `json:"scope_type"`
	SiteID         *int64 `json:"site_id,omitempty"`
	GateID         *int64 `json:"gate_id,omitempty"`
	IsActive       bool   `json:"is_active"`
	RoleName       string `json:"role_name"`
	RoleCode       string `json:"role_code"`
}
