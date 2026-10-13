package identificationtypes

type CreateInput struct {
	Code              string `json:"code"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	ValidationRegex   string `json:"validation_regex"`
	ValidationExample string `json:"validation_example"`
	IsActive          bool   `json:"is_active"`
	IsDefault         bool   `json:"is_default"`
}

type UpdateInput struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	ValidationRegex   string `json:"validation_regex"`
	ValidationExample string `json:"validation_example"`
	IsActive          *bool  `json:"is_active"`
	IsDefault         *bool  `json:"is_default"`
}
