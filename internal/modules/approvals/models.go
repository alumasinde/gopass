package approvals

type ApprovalRequest struct {
	ID int64 `json:"id"`
	OrganizationID int64 `json:"organization_id"`
	GatepassID int64 `json:"gatepass_id"`
	Status string `json:"status"`
	StepOrder int `json:"step_order"`
	ActedBy *int64 `json:"acted_by,omitempty"`
}
