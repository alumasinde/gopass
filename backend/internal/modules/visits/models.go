package visits

import "time"

type Visit struct {
	ID             int64     `json:"id"`
	OrganizationID int64     `json:"organization_id"`
	VisitorID      int64     `json:"visitor_id"`
	SiteID         int64     `json:"site_id"`
	GateID         int64     `json:"gate_id"`
	Direction      string    `json:"direction"` // "IN" or "OUT"
	VisitDate      string    `json:"visit_date"`
	CheckInTime    string    `json:"check_in_time"`
	CheckOutTime   *string   `json:"check_out_time,omitempty"`
	Purpose        string    `json:"purpose"`
	HostName       string    `json:"host_name"`
	HostDepartment string    `json:"host_department"`
	IsBlacklisted  bool      `json:"is_blacklisted"`
	BlacklistReason *string  `json:"blacklist_reason,omitempty"`
	Notes          string    `json:"notes"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
