// Package apperr is the single place where every API error is defined.
//
// Each error carries its HTTP status, a stable machine-readable code and a
// default message. Handlers, services and middleware return or send these
// values instead of hand-writing status codes and strings.
package apperr

import (
	"errors"
	"net/http"
)

// Error is an API error with a transport status, a stable code and a message.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// With returns a copy of the error with a more specific message.
// The status and code are unchanged, so errors.Is still matches the original.
func (e *Error) With(msg string) *Error {
	c := *e
	c.Message = msg
	return &c
}

// Is makes errors.Is(err, apperr.NotFound) true for any copy made by With.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

// The error catalogue. Add new errors here and nowhere else.
var (
	InvalidRequest = &Error{http.StatusBadRequest, "invalid_request", "invalid request"}
	InvalidJSON    = &Error{http.StatusBadRequest, "invalid_request", "invalid JSON"}
	InvalidID      = &Error{http.StatusBadRequest, "invalid_id", "invalid id"}
	Unauthorized   = &Error{http.StatusUnauthorized, "unauthorized", "authentication required"}
	InvalidToken   = &Error{http.StatusUnauthorized, "invalid_token", "invalid or expired access token"}
	Forbidden      = &Error{http.StatusForbidden, "forbidden", "permission denied"}
	NotFound       = &Error{http.StatusNotFound, "not_found", "resource not found"}
	Conflict       = &Error{http.StatusConflict, "conflict", "resource could not be saved"}
	CreateFailed   = &Error{http.StatusConflict, "conflict", "resource could not be created"}
	Database       = &Error{http.StatusInternalServerError, "database_error", "database error"}
	TenantMissing  = &Error{http.StatusInternalServerError, "tenant_missing", "organization context missing"}
	Internal       = &Error{http.StatusInternalServerError, "internal_error", "internal server error"}
)

// From converts any error into an *Error. Unknown errors become Internal so
// that raw error text is never leaked to clients.
func From(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return Internal
}
