package roles

import "errors"

var (
	ErrNotFound = errors.New("roles: resource not found")
	ErrInvalid  = errors.New("roles: invalid request")
)
