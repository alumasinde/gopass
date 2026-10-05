package organizations

import "context"

// Repository is the persistence boundary for organizations.
// Tenant-owned operations must receive tenant context before querying MySQL.
type Repository interface {
	Ping(ctx context.Context) error
}
