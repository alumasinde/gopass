package credentials

type Credential struct {
	ID             int64  `json:"id"`
	OrganizationID int64  `json:"organization_id"`
	GatepassID     int64  `json:"gatepass_id"`
	Token          string `json:"token"`
	ExpiresAt      string `json:"expires_at"`
	IsRevoked      bool   `json:"is_revoked"`
}
