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

Example dedicated test DSN (never use a production DSN):

```powershell
$env:DB_DSN='testuser:testpass@tcp(127.0.0.1:3306)/gopass_test?parseTime=true&charset=utf8mb4'
go run ./cmd/migrate up
go test ./... -count=1
```

The current automated unit tests do not automatically create or migrate this database; the DSN commands are preparation guidance for the integration-test phase.
