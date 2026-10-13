package approvals

import "context"

// Repository is the persistence boundary for approvals.
// Tenant-owned operations must receive tenant context before querying MySQL.
type Repository interface {
	Ping(ctx context.Context) error
}
