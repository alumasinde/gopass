package visitors

import (
	"strings"
)

type Input struct {
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	CompanyName  string `json:"company_name"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
	IDTypeID     int64  `json:"id_type_id"`
	IDNumber     string `json:"id_number"`
	IDExpiryDate string `json:"id_expiry_date"`
}

func (in *Input) normalize() {
	in.FirstName = strings.TrimSpace(in.FirstName)
	in.LastName = strings.TrimSpace(in.LastName)
	in.CompanyName = strings.TrimSpace(in.CompanyName)
	in.Phone = strings.TrimSpace(in.Phone)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.IDNumber = strings.ToUpper(strings.TrimSpace(in.IDNumber))
	in.IDExpiryDate = strings.TrimSpace(in.IDExpiryDate)
}
