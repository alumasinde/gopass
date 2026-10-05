package gatepasses

import "errors"


var (
	ErrNotFound         = errors.New("gatepass not found")
	ErrInvalid          = errors.New("invalid gatepass")
	ErrTransition       = errors.New("invalid gatepass transition")
	ErrBlacklisted      = errors.New("visitor is blacklisted")
	ErrApprovalRequired = errors.New("approval workflow not configured")
)