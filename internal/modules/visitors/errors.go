package visitors

import "errors"

var (
	ErrNotFound = errors.New("visitors: resource not found")
	ErrInvalid  = errors.New("visitors: invalid request")
)
