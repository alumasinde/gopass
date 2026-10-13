# Platform administration and notification delivery

## Apply migration

Back up MySQL, then apply the new additive migration. Migrations `0001`–`0008` are not edited.

```powershell
go run ./cmd/migrate status
go run ./cmd/migrate up
```

Migration `0009_notifications_platform_tenants.sql` adds the durable notification queue, platform administrator/session/audit tables, and tenant lifecycle/provisioning metadata. Existing organizations remain `ACTIVE`/`READY` by default.

## Bootstrap the first platform administrator

Set environment variables in the shell (do not commit secrets):

```powershell
$env:PLATFORM_ADMIN_EMAIL = "platform-admin@example.com"
$env:PLATFORM_ADMIN_PASSWORD = "a-long-unique-password-of-14-chars-minimum"
$env:PLATFORM_ADMIN_NAME = "GoPass Platform Administrator"
go run ./cmd/platform-admin-create
```

The command hashes the password with bcrypt. It never prints the password. Run after migration 0009. Duplicate email is rejected by the unique index.

## Platform API

Base path: `/api/v1/platform`

- `POST /auth/login` — platform email/password login; returns a short-lived access token.
- `POST /auth/logout` — revoke current platform session.
- `GET /admins`, `POST /admins`, `PATCH /admins/{id}/suspend` — list, create, and suspend platform administrators; suspending an admin revokes their sessions.
- `GET /organizations` and `GET /organizations/{id}` — tenant lifecycle and provisioning status.
- `POST /organizations` — create a shared-database organization and its initial organization-admin user atomically.
- `PATCH /organizations/{id}/suspend` — immediately disables tenant login/access.
- `PATCH /organizations/{id}/resume` — only a READY shared tenant can be resumed.
- `POST /organizations/{id}/provision/retry` — retries failed shared-database provisioning.
- `POST /organizations/{id}/database-config` — encrypts and stores a dedicated database DSN, but intentionally leaves the tenant inactive and BLOCKED until dedicated DB routing and tenant-specific migrations are implemented.
- `POST /organizations/{id}/deprovision` — marks a tenant deprovisioned without physically deleting business data.

All lifecycle mutations and platform login/logout session events are written to `platform_audit_logs` in the same transaction as their database mutation. Platform tokens use a distinct `prn=platform` claim, have no organization ID, and are accepted only by platform middleware backed by an active platform session. Tenant middleware rejects platform tokens.

### Tenant creation payload

```json
{
  "name": "Example Company",
  "slug": "example-company",
  "database_mode": "SHARED",
  "admin_first_name": "Jane",
  "admin_last_name": "Admin",
  "admin_email": "jane@example.com",
  "admin_password": "a-long-unique-password-of-14-chars-minimum"
}
```

Shared tenants begin `PROVISIONING/PENDING`; the create operation atomically creates the initial organization-admin user and grants the tenant’s current permission catalogue. The background provisioner verifies the shared schema and then marks them `ACTIVE/READY`. A tenant remains inactive until ready.

## Dedicated database limitation

Dedicated DSNs are encrypted using AES-256-GCM before being stored. Set `PLATFORM_DB_ENCRYPTION_KEY` to a securely generated 32-byte value, and keep the same key backed up outside the database and source control. The current GoPass data plane still uses one shared `*sql.DB`, and the migration set is not split into platform vs tenant schemas. For that reason the endpoint **does not activate a dedicated tenant**; it records `DEDICATED/BLOCKED`. Do not manually flip this state to READY. Complete a tenant-aware DB router and tenant migration runner before enabling dedicated tenants.

## Notification delivery

Workflow events enqueue an email notification inside the same SQL transaction as the state change. A rollback therefore removes both the business change and its queued notification. Events include:

- `approval.requested`
- `approval.approved`
- `approval.rejected`
- `gatepass.submitted`
- `gatepass.issued`
- `visitor.arrived`
- `visitor.departed`

Notifications with no valid visitor email are skipped. Delivery is asynchronous through the background worker with at-least-once semantics (a process crash after SMTP acceptance but before recording success can cause a duplicate email). Configure `SMTP_HOST`, `SMTP_PORT`, `SMTP_FROM`, and optionally `SMTP_USERNAME`/`SMTP_PASSWORD`. Failed sends use exponential backoff (30 seconds up to six hours), are retried up to eight attempts, and then enter `DEAD`. Stale PROCESSING rows are recovered after ten minutes. The worker does not log email body/payload or SMTP credentials.

Tenant delivery status endpoints:

- `GET /api/v1/notifications` — last 100 notifications for the authenticated organization.
- `POST /api/v1/notifications/{id}/retry` — retries a dead-letter notification; requires `notifications.retry`.

Migration 0009 adds these permissions and assigns them to existing `organization_admin` roles. New tenants receive all current permission catalogue entries through their initial organization-admin role.

## Verification

```powershell
go test ./...
go test -tags=integration ./...
go vet ./...
```

Race detector on Windows (MSYS2 UCRT64 GCC configured):

```powershell
$env:CGO_ENABLED = "1"
$env:CC = "C:\msys64\ucrt64\bin\gcc.exe"
$env:PATH = "C:\msys64\ucrt64\bin;" + $env:PATH
go test -race ./...
```

Use a dedicated test database for integration tests. The build environment that produced this archive had no network access to the Go module proxy, so the tests need to be executed locally before production deployment.
