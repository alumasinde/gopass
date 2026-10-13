package visits

import "errors"

var (
	ErrNotFound      = errors.New("visit not found")
	ErrInvalid       = errors.New("invalid visit")
	ErrAlreadyClosed = errors.New("visit already checked out")
	ErrBlacklisted   = errors.New("visitor is blacklisted")
)
