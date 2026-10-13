# Authorization and Tenant Database Routing

## Fail-closed authorization

Tenant-owned module repositories use `internal/app/database.Handle`. The production `database.Router` uses `PLATFORM_DB_DSN` as the authoritative control-plane registry and `DB_DSN` as the shared tenant data plane. It requires an organization ID in request context and checks the platform registry before every routed database operation. Missing tenant context, inactive tenants, non-READY provisioning state, invalid encrypted DSNs, and unavailable dedicated databases fail closed.

Platform-control-plane handlers use the platform `*sql.DB` explicitly. The tenant bootstrap queue stores only a short-lived password hash and initial identity payload until it can create the user/role in the tenant data plane; the request is removed after bootstrap succeeds. Public login resolves organization metadata through `CentralHandle`, then performs user/password/session operations with the organization bound to context. Refresh/logout bind the signed organization claim before touching session storage.

`ORGANIZATION` permissions remain organization-wide. `SITE` and `GATE` permissions are filtered for site/gate lists and for gatepass, approval, credential, check-in and check-out list/detail operations. Visitors, roles, users, pass types and organization settings remain organization-scoped because those resources are not safely partitioned by a gate without a separate ownership model.

## Shared and dedicated tenant modes

- `SHARED`: tenant-owned queries route to the shared tenant database and always retain organization predicates.
- `DEDICATED`: tenant-owned queries route to the database encrypted in the platform registry’s `organizations.database_config_ciphertext`.
- The router rejects missing tenant context and requires platform-registry lifecycle `ACTIVE`, `is_active=1`, and provisioning `READY` before routing.
- Dedicated DB credentials are encrypted with AES-GCM and `PLATFORM_DB_ENCRYPTION_KEY`; the same key must be configured consistently across API and worker instances.
- The target DSN must point to an existing MySQL database. The platform validates connectivity and rejects both the platform and shared schemas and database targets already assigned to another tenant.
- Provisioning applies the embedded migrations in order, validates required tables and columns, rejects unrelated/non-empty schemas, seeds the organization, and copies tenant users/roles/role-permission assignments before central status is switched to `READY`/`ACTIVE`.
- Failed provisioning leaves the tenant inactive. Retry is explicit through the platform provisioning retry endpoint.
- Suspending or deprovisioning a tenant blocks routing centrally, revokes refresh sessions in the tenant's actual database, and invalidates cached connection pools. Resuming is blocked if session revocation cannot be completed.

## Dedicated database setup

1. Create an empty database on the target MySQL server and a least-privilege DB user with schema migration privileges.
2. Set `PLATFORM_DB_ENCRYPTION_KEY` to exactly 32 bytes or base64-encoded 32 bytes.
3. Create the tenant with `database_mode=DEDICATED` or configure an existing tenant through the platform database-config endpoint.
4. Supply a MySQL DSN including the database name. Passwords are never written to application logs or returned by the API.
5. The background tenant provisioner migrates and validates the schema. A tenant is not active until provisioning reaches `READY`.
6. Confirm the organization shows `ACTIVE` and `READY`, then log in again. Existing shared-database refresh sessions are intentionally revoked during a database-mode change.

## Test commands

```powershell
go test ./...
go test -tags=integration -v ./internal/security/integration
go test -tags=integration ./...
go vet -tags=integration ./...
```

For dedicated database routing integration tests, configure both `GOPASS_TEST_DSN` and `GOPASS_DEDICATED_TEST_DSN`. Both must point to separate disposable databases ending in `_test`; the dedicated test schema must not contain another organization. The test also requires `PLATFORM_DB_ENCRYPTION_KEY`.

## Operational cautions

- Do not give tenant DB credentials global administrative privileges. Use a dedicated DB user per tenant where possible.
- Back up the platform control-plane database, shared tenant database and each dedicated tenant database before changing provisioning or migration logic.
- The platform organization record is the authority for activation/suspension; dedicated database state alone never makes a tenant routable.
- Database transactions cannot be atomic across the central control-plane DB and a tenant DB. Lifecycle operations are ordered fail-closed: suspension commits centrally first, and a failed session-revocation attempt leaves the tenant suspended. A resume revokes sessions before reactivation.
