# Fixes from the first MySQL security integration run

This patch addresses three defects exposed by the first run:

1. `internal/security/integration/security_integration_test.go`: role fixtures now pass `organization_id` to the three-placeholder INSERT statement.
2. `internal/security/integration/security_integration_test.go`: pass-type fixtures now pass `organization_id` to the three-placeholder INSERT statement.
3. `internal/modules/users/handlers.go`: public login now resolves organization ID/slug without accidentally retaining the missing-tenant-context error. A valid explicit `organization_id` is accepted; slug lookup failures still fail closed.

No migrations were changed. Migrations `0001` through `0007` remain the expected schema history.

## Verify on the development machine

```powershell
gofmt -w .\internal\modules\users\handlers.go .\internal\security\integration\security_integration_test.go
go test ./...
go test -tags=integration -v ./internal/security/integration
go test -tags=integration ./...
go vet -tags=integration ./...
```

Set `GOPASS_TEST_DSN` to a dedicated MySQL test database whose database name ends in `_test` before running the integration suite. The test suite is destructive only within its own generated fixture rows and refuses DSNs whose database name does not end in `_test`.
