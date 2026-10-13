package sites

import "errors"

var (
	ErrNotFound = errors.New("site not found")
	ErrExists   = errors.New("site already exists")
	ErrInvalid  = errors.New("invalid site")
)
