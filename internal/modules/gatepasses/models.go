package gatepasses

type Gatepass struct {
	ID             int64  `json:"id"`
	OrganizationID int64  `json:"organization_id"`
	VisitorID      int64  `json:"visitor_id"`
	GateID         int64  `json:"gate_id"`
	PassTypeID     int64  `json:"pass_type_id"`
	Status         string `json:"status"`
	ValidFrom      string `json:"valid_from"`
	ValidUntil     string `json:"valid_until"`
}
