package users

import "testing"

func TestNormalizeTenantHost(t *testing.T) {
	valid := map[string]string{
		"visitors.example.com":     "visitors.example.com",
		"VISITORS.EXAMPLE.COM.":    "visitors.example.com",
		"visitors.example.com:443": "visitors.example.com",
	}
	for input, want := range valid {
		got, err := normalizeTenantHost(input)
		if err != nil || got != want {
			t.Errorf("normalizeTenantHost(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", "https://example.com", "example.com/path", "localhost", "127.0.0.1", "bad..example.com", "-bad.example.com"} {
		if got, err := normalizeTenantHost(input); err == nil {
			t.Errorf("normalizeTenantHost(%q) unexpectedly accepted %q", input, got)
		}
	}
}
