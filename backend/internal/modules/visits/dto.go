package visits

type CreateInput struct {
	VisitorID      int64  `json:"visitor_id"`
	SiteID         int64  `json:"site_id"`
	GateID         int64  `json:"gate_id"`
	Direction      string `json:"direction"` // "IN" or "OUT"
	Purpose        string `json:"purpose"`
	HostName       string `json:"host_name"`
	HostDepartment string `json:"host_department"`
	Notes          string `json:"notes"`
}

type UpdateInput struct {
	CheckOutTime   *string `json:"check_out_time"`
	Purpose        string  `json:"purpose"`
	HostName       string  `json:"host_name"`
	HostDepartment string  `json:"host_department"`
	Notes          string  `json:"notes"`
}

type CheckInInput struct {
	VisitorID      int64  `json:"visitor_id"`
	SiteID         int64  `json:"site_id"`
	GateID         int64  `json:"gate_id"`
	Purpose        string `json:"purpose"`
	HostName       string `json:"host_name"`
	HostDepartment string `json:"host_department"`
}

type CheckOutInput struct {
	VisitID int64 `json:"visit_id"`
}
