package gates

import "errors"

var (
	ErrNotFound = errors.New("gate not found")
	ErrInvalid  = errors.New("invalid gate")
	ErrExists   = errors.New("gate exists")
)