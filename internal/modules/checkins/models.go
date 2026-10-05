package checkins

type CheckIn struct {
	ID int64 `json:"id"`
	OrganizationID int64 `json:"organization_id"`
	GatepassID int64 `json:"gatepass_id"`
	GateID int64 `json:"gate_id"`
	CheckedInBy *int64 `json:"checked_in_by,omitempty"`
	CheckedInAt string `json:"checked_in_at"`
}
