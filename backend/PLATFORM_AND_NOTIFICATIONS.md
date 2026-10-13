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
- `POST /organizations` — create a SHARED or DEDICATED organization and its initial organization-admin user atomically; dedicated tenants remain inactive until database configuration and provisioning complete.
- `PATCH /organizations/{id}/suspend` — immediately disables tenant login/access.
- `PATCH /organizations/{id}/resume` — only a READY tenant can be resumed; old refresh sessions are revoked before reactivation.
- `POST /organizations/{id}/provision/retry` — retries failed provisioning for shared or configured dedicated tenants.
- `POST /organizations/{id}/database-config` — validates target connectivity/schema isolation, encrypts and stores the dedicated database DSN, then queues provisioning. The tenant stays inactive until migrations and schema validation succeed.
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

## Dedicated database provisioning and routing

Dedicated database routing is implemented in `internal/app/database/router.go`. Tenant-owned repositories use the router and require an organization-bound request context. The router checks the central organization lifecycle on each routed database operation and refuses missing tenant context, suspended/deprovisioned tenants, non-READY provisioning states, and unavailable or invalid dedicated databases.

Dedicated DSNs are encrypted using AES-256-GCM. Set `PLATFORM_DB_ENCRYPTION_KEY` to a securely generated 32-byte value (or base64-encoded 32-byte key) and keep it identical across API and worker instances. The platform rejects the central database and a target already assigned to another dedicated tenant. The target database must already exist and the supplied DB user must have permission to apply schema migrations.

The background provisioner runs embedded migrations `0001`–`0009` against the dedicated database, validates required tables/columns/indexes, rejects unrelated organization records, seeds the organization row and copies the initial tenant users, roles and role-permission grants. Only after this succeeds does the central control plane switch the tenant to `ACTIVE/READY`. Existing refresh sessions are intentionally not copied to the dedicated database when a tenant changes database mode.

Suspension blocks routing centrally, revokes refresh sessions in the tenant's actual database and invalidates the cached connection pool. If the dedicated database is unavailable, suspension remains in effect; resumption is blocked until session revocation can complete. Provisioning failures leave the tenant inactive with an error status and can be retried through the platform endpoint.

The dedicated database integration test is opt-in. Configure `GOPASS_TEST_DSN` and `GOPASS_DEDICATED_TEST_DSN` to separate disposable MySQL databases ending in `_test`, plus `PLATFORM_DB_ENCRYPTION_KEY`. Do not use production databases for these tests.

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

Use a dedicated test database for integration tests. Run the full test suite and dedicated database integration test locally before production deployment; dedicated database tests are skipped unless their separate test DSN and encryption key are configured.
