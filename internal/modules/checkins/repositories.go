package checkins

import "context"

// Repository is the persistence boundary for checkins.
// Tenant-owned operations must receive tenant context before querying MySQL.
type Repository interface {
	Ping(ctx context.Context) error
}
