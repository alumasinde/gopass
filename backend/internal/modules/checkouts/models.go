package checkouts

type CheckOut struct {
	ID             int64  `json:"id"`
	OrganizationID int64  `json:"organization_id"`
	GatepassID     int64  `json:"gatepass_id"`
	GateID         int64  `json:"gate_id"`
	CheckedOutBy   *int64 `json:"checked_out_by,omitempty"`
	CheckedOutAt   string `json:"checked_out_at"`
}
