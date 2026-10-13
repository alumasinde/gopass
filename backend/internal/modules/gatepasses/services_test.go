package gatepasses

import "testing"

func TestValidTransition(t *testing.T) {
	tests := []struct {
		from, to string
		want     bool
	}{
		{StatusDraft, StatusCancelled, true},
		{StatusDraft, StatusApproved, false},
		{StatusPendingApproval, StatusApproved, false},
		{StatusPendingApproval, StatusRejected, false}, // approvals service owns this transition
		{StatusApproved, StatusIssued, true},
		{StatusApproved, StatusCheckedIn, false},
		{StatusApproved, StatusCancelled, true},
		{StatusApproved, StatusRevoked, true},
		{StatusIssued, StatusCheckedIn, true},
		{StatusIssued, StatusCheckedOut, false},
		{StatusIssued, StatusRevoked, true},
		{StatusCheckedIn, StatusCheckedOut, true},
		{StatusCheckedIn, StatusIssued, false},
		{StatusCheckedOut, StatusIssued, false},
		{StatusCheckedOut, StatusCheckedIn, false},
		{StatusRejected, StatusIssued, false},
		{StatusCancelled, StatusIssued, false},
		{StatusRevoked, StatusIssued, false},
	}
	for _, tt := range tests {
		if got := validTransition(tt.from, tt.to); got != tt.want {
			t.Errorf("%s -> %s = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}
