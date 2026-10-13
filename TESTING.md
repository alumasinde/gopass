# Testing

## Unit/API
Run `go test ./...` after dependencies are available. The suite should include service, handler, authorization, tenant-isolation and workflow integration tests.

## MySQL integration
Use a dedicated database and DSN; never point tests at development/production data. Example:

```powershell
$env:DB_DSN='testuser:testpass@tcp(127.0.0.1:3306)/gopass_test?parseTime=true&charset=utf8mb4'
go test ./... -count=1
```
## Migrations
Apply the schema to the test database before running integration tests:

```powershell
go run ./cmd/migrate up
```

## Required authorization cases
- Tenant A cannot read Tenant B resources.
- Security Officer cannot manage roles/users.
- Receptionist cannot perform security overrides.
- Organization Administrator cannot access platform-only operations.
- Site-scoped users cannot read resources outside their site.
- Gate-scoped users cannot operate another gate.

## Session revocation and scope regression cases
- Login issues access and refresh tokens with the same session ID.
- Refresh rotation returns both tokens, keeps the same session ID, and consumes the old refresh token exactly once.
- Logout rejects a stale/already-consumed refresh token and revokes the current session; an existing access token from that session must then receive `401`.
- Disabling a user or organization invalidates existing access tokens and prevents refresh.
- `Require` must not accept permissions granted only at `SITE` or `GATE` scope.
- `RequireWithScope` must allow only organization/site/gate resources that belong to the caller's organization and match the assigned scope.
- Assigning a role with `SITE` scope requires a valid same-organization `site_id`; `GATE` scope requires a valid same-organization `gate_id`; invalid/mixed scope combinations must fail.
- Role assignment must not create a user-role row if the user belongs to another organization.
- Role/permission assignments and removals, login, refresh, and logout should create audit rows with organization and actor where available.
