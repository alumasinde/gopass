package visitors

import "errors"

var (
	ErrNotFound          = errors.New("visitor not found")
	ErrInvalid           = errors.New("invalid visitor")
	ErrDuplicateIdentity = errors.New("identification number already exists for this identification type")
)
