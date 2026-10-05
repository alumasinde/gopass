package visitors

import "errors"

var (
	ErrNotFound = errors.New("visitor not found")
	ErrInvalid  = errors.New("invalid visitor")
)