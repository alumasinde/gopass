package approvals

import "errors"

var (
	ErrNotFound = errors.New("approvals: resource not found")
	ErrInvalid  = errors.New("approvals: invalid request")
)
