package checkouts

import "errors"


var (
	ErrInvalid  = errors.New("invalid check-out")
	ErrNotFound = errors.New("active check-in not found")
)