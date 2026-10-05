package passtypes

import "errors"

var (
	ErrNotFound = errors.New("passtypes: resource not found")
	ErrInvalid  = errors.New("passtypes: invalid request")
)
