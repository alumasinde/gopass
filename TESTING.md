# Testing and Security Regression Suite

Run these commands from the repository root after dependencies are available:

```powershell
go test ./...
go test -race ./...
go vet ./...
```

On Windows, the race detector requires CGO and a working C compiler. For the MSYS2 UCRT64 toolchain:

```powershell
$env:CGO_ENABLED = "1"
$env:CC = "C:\msys64\ucrt64\bin\gcc.exe"
$env:PATH = "C:\msys64\ucrt64\bin;" + $env:PATH
go test -race ./...
```

## Unit coverage added

- `internal/app/http/middleware`: malformed bearer headers, inactive users, revoked sessions, lookup failures, authenticated context propagation, security headers and CORS origin rejection.
- `internal/platform/rbac`: fail-closed behavior without claims, organization-claim/context mismatch, invalid scope types and invalid site IDs.
- `internal/platform/ratelimit`: per-email and per-IP limits, successful-login reset behavior and concurrent access.
- `internal/platform/audit`: authenticated tenant/actor precedence and JSON serialization of old/new values.
- `internal/modules/approvals`: unauthenticated approval actions are denied before database access.
- `internal/modules/checkins`: unauthenticated check-in denial and credential hash properties.
- `internal/modules/checkouts`: unauthenticated check-out denial.
- `internal/modules/credentials`: unauthenticated credential verification denial and token hash properties.
- `internal/modules/users`: refresh-token rotation, concurrent refresh use, logout/session revocation and login rate-limiter initialization.
- `internal/modules/gatepasses`: state-transition matrix.

## MySQL integration tests still required

The unit suite does not replace integration tests against a dedicated MySQL test database. Do not run security tests against production data. Apply migrations to an isolated database and verify these scenarios through the HTTP API and database:

1. Organization A cannot read, update, delete, approve, check in, check out, or issue/verify credentials for Organization B resources.
2. A site-scoped user cannot act on another site; a gate-scoped user cannot act on another gate.
3. `Require` does not grant organization-wide permission based solely on site/gate scope.
4. Role assignments reject users, roles, sites, or gates owned by another organization and reject missing/mixed scope identifiers.
5. Logout immediately invalidates an existing access token; refresh-token replay fails; only one concurrent refresh succeeds.
6. Disabling a user or organization invalidates active sessions.
7. Role/permission changes and authentication/session events create expected audit records with the correct organization and actor.
8. Concurrent check-in/check-out/approval requests do not create duplicate records or skip valid workflow transitions.
9. Migration `0008_workflow_integrity.sql` adds tenant-consistent composite foreign keys and database uniqueness guards; investigate and repair existing cross-tenant references or duplicate active records if it refuses to apply.

Example dedicated test DSN (never use a production DSN):

```powershell
$env:DB_DSN='testuser:testpass@tcp(127.0.0.1:3306)/gopass_test?parseTime=true&charset=utf8mb4'
go run ./cmd/migrate up
go test ./... -count=1
```

The current automated unit tests do not automatically create or migrate this database; the DSN commands are preparation guidance for the integration-test phase.

## MySQL-backed security integration tests

The `internal/security/integration` suite runs the real GoPass HTTP router against MySQL. It is deliberately behind the `integration` build tag and refuses to run against a database whose name does not end in `_test`. It creates uniquely named organizations and deletes only the fixture rows it creates.

### 1. Create and migrate a dedicated test database

Do not point these tests at development, staging, or production data. Create a database such as `gopass_test` and grant the local test account access to it. In PowerShell, set the DSN to that database:

```powershell
$env:DB_DSN = "gopass_user:YOUR_TEST_PASSWORD@tcp(127.0.0.1:3306)/gopass_test?parseTime=true"
$env:GOPASS_TEST_DSN = $env:DB_DSN

go run ./cmd/migrate status
go run ./cmd/migrate up
```

The test harness checks that migration `0008_workflow_integrity.sql` is recorded before it runs. Existing migration files are not modified by this test suite.

### 2. Run integration security tests

```powershell
go test -tags=integration -v ./internal/security/integration
```

Or run all unit and integration tests, then static analysis:

```powershell
go test -tags=integration ./...
go test -tags=integration -race ./...
go vet -tags=integration ./...
```

The race test on Windows requires CGO and a working compiler. With the MSYS2 UCRT64 toolchain used for this project:

```powershell
$env:CGO_ENABLED = "1"
$env:CC = "C:\msys64\ucrt64\bin\gcc.exe"
$env:PATH = "C:\msys64\ucrt64\bin;" + $env:PATH
go test -tags=integration -race ./...
```

Unset the test DSN after testing if you do not want later commands to use it:

```powershell
Remove-Item Env:GOPASS_TEST_DSN -ErrorAction SilentlyContinue
Remove-Item Env:DB_DSN -ErrorAction SilentlyContinue
```

### 3. Security assertions covered

- Cross-organization site read, update, and delete attempts return `404`; the foreign row remains unchanged and active in MySQL.
- Site-scoped roles can access gatepasses at their assigned site, but not at another site.
- Gate-scoped roles can access gatepasses at their assigned gate, but not at another gate, including a different gate on the same site.
- Logout revokes the refresh-token session and the previously issued access token is immediately rejected by the protected API.
- Refresh-token rotation returns a new access/refresh pair; replaying the old refresh token is rejected and the replacement remains usable.
- Role assignment/removal and permission assignment/removal produce persistent `audit_logs` rows attributed to the organization and actor.
- Database constraints reject cross-organization foreign-key references, duplicate check-in/check-out rows, multiple active credentials per gatepass, and multiple pending approval requests per gatepass.
- Gatepass create/submit/cancel transitions produce transactional audit rows with old/new state snapshots.

These tests use real MySQL rows and the actual route/middleware/handler stack. They complement the fast unit tests; do not replace them.
