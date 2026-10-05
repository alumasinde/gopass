package approvals

import "errors"

var (
	ErrNotFound     = errors.New("approval request not found")
	ErrInvalid      = errors.New("invalid approval request")
	ErrAlreadyActed = errors.New("approval already acted")
	ErrForbidden    = errors.New("not eligible to approve")
)