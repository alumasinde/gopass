package gatepasses

import "errors"

var (
	ErrNotFound = errors.New("gatepasses: resource not found")
	ErrInvalid  = errors.New("gatepasses: invalid request")
)
