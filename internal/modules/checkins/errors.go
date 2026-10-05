package checkins

import "errors"

var (
	ErrInvalid  = errors.New("invalid check-in")
	ErrNotFound = errors.New("gatepass or credential not found")
	ErrAlready  = errors.New("already checked in")
)