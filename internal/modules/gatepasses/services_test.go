package gatepasses

import "testing"

func TestValidTransition(t *testing.T) {
	tests := []struct {
		from, to string
		want     bool
	}{
		{StatusDraft, StatusCancelled, true}, {StatusDraft, StatusApproved, false},
		{StatusApproved, StatusIssued, true}, {StatusApproved, StatusCheckedIn, false},
		{StatusIssued, StatusCheckedIn, true}, {StatusIssued, StatusCheckedOut, false},
		{StatusCheckedIn, StatusCheckedOut, true}, {StatusCheckedIn, StatusIssued, false},
		{StatusCheckedOut, StatusIssued, false}, {StatusRejected, StatusIssued, false},
	}
	for _, tt := range tests {
		if got := validTransition(tt.from, tt.to); got != tt.want {
			t.Errorf("%s -> %s = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}
