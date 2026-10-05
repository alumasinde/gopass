package sites

import "errors"

var (
	ErrNotFound = errors.New("sites: resource not found")
	ErrInvalid  = errors.New("sites: invalid request")
)
