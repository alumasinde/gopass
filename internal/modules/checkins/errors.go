package checkins

import "errors"

var (
	ErrNotFound = errors.New("checkins: resource not found")
	ErrInvalid  = errors.New("checkins: invalid request")
)
