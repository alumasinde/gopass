package credentials

import "errors"

var (
	ErrNotFound    = errors.New("credential not found")
	ErrInvalid     = errors.New("invalid credential")
	ErrUnavailable = errors.New("credential unavailable")
)