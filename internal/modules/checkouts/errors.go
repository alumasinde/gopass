package checkouts

import "errors"

var (
	ErrNotFound = errors.New("checkouts: resource not found")
	ErrInvalid  = errors.New("checkouts: invalid request")
)
