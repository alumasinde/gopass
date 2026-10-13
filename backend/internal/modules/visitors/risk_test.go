package visitors

import (
	"testing"
	"time"
)

func TestCalculateRiskMissingContactAndIdentity(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	score, level, factors := calculateRisk(Visitor{FirstName: "Jane", LastName: "Doe"}, now)
	if score != 70 { // phone 20 + email 15 + identity 30 + company 5
		t.Fatalf("score = %d, want 70", score)
	}
	if level != "HIGH" {
		t.Fatalf("level = %q, want HIGH", level)
	}
	if len(factors) != 4 {
		t.Fatalf("factors = %d, want 4", len(factors))
	}
}

func TestCalculateRiskWithContactAndValidIdentity(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	idType := int64(2)
	score, level, factors := calculateRisk(Visitor{Phone: "0712345678", Email: "jane@example.com", CompanyName: "Example Ltd", IDTypeID: &idType, IDNumber: "A12345678"}, now)
	if score != 0 || level != "LOW" || len(factors) != 1 {
		t.Fatalf("got score=%d level=%s factors=%v; want 0 LOW and no-risk factor", score, level, factors)
	}
}

func TestCalculateRiskExpiredIDAndBlacklist(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	idType := int64(1)
	v := Visitor{Phone: "1", Email: "a@b.test", CompanyName: "Company", IDTypeID: &idType, IDNumber: "A123", IDExpiryDate: "2025-01-01"}
	score, level, _ := calculateRisk(v, now)
	if score != 15 || level != "LOW" {
		t.Fatalf("expired ID got score=%d level=%s", score, level)
	}
	v.IsBlacklisted = true
	score, level, factors := calculateRisk(v, now)
	if score != 100 || level != "CRITICAL" || len(factors) < 2 {
		t.Fatalf("blacklist got score=%d level=%s factors=%v", score, level, factors)
	}
}
