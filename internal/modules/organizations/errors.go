package organizations

import "errors"

var (
	ErrNotFound = errors.New("organizations: resource not found")
	ErrInvalid  = errors.New("organizations: invalid request")
)
