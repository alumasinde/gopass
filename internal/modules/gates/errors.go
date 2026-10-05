package gates

import "errors"

var (
	ErrNotFound = errors.New("gates: resource not found")
	ErrInvalid  = errors.New("gates: invalid request")
)
