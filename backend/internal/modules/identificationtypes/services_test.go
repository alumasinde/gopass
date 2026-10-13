package identificationtypes

import "testing"

func TestValidateIdentificationType(t *testing.T) {
	good := &IdentificationType{Code: "NATIONAL_ID", Name: "National ID", IsActive: true, ValidationRegex: `^[A-Z0-9-]{4,30}$`}
	if err := validate(good); err != nil {
		t.Fatalf("valid type rejected: %v", err)
	}
	badCode := &IdentificationType{Code: "national id", Name: "National ID"}
	if err := validate(badCode); err == nil {
		t.Fatal("invalid code accepted")
	}
	badRegex := &IdentificationType{Code: "OTHER", Name: "Other", ValidationRegex: "["}
	if err := validate(badRegex); err == nil {
		t.Fatal("invalid validation regex accepted")
	}
}
