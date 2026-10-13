package notifications

import "testing"

func TestBackoffSeconds(t *testing.T) {
	tests := []struct{ attempt, want int }{{1, 30}, {2, 60}, {3, 120}, {10, 15360}, {20, 21600}}
	for _, tt := range tests {
		if got := backoffSeconds(tt.attempt); got != tt.want {
			t.Fatalf("backoffSeconds(%d)=%d want %d", tt.attempt, got, tt.want)
		}
	}
}
