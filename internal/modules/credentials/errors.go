package credentials

import "errors"

var (
	ErrNotFound = errors.New("credentials: resource not found")
	ErrInvalid  = errors.New("credentials: invalid request")
)
