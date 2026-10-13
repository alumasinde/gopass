package visitors

type Visitor struct {
	ID             int64    `json:"id"`
	OrganizationID int64    `json:"organization_id"`
	FirstName      string   `json:"first_name"`
	LastName       string   `json:"last_name"`
	CompanyName    string   `json:"company_name,omitempty"`
	Phone          string   `json:"phone"`
	Email          string   `json:"email"`
	IDTypeID       *int64   `json:"id_type_id,omitempty"`
	IDTypeCode     string   `json:"id_type_code,omitempty"`
	IDTypeName     string   `json:"id_type_name,omitempty"`
	IDNumber       string   `json:"id_number,omitempty"`
	IDExpiryDate   string   `json:"id_expiry_date,omitempty"`
	IsBlacklisted  bool     `json:"is_blacklisted"`
	RiskScore      int      `json:"risk_score"`
	RiskLevel      string   `json:"risk_level"`
	RiskFactors    []string `json:"risk_factors"`
}
