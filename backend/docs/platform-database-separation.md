# GoPass platform database separation

GoPass uses two database roles:

- **Platform control plane (`PLATFORM_DB_DSN`)**: `organizations` registry, `platform_users`, `platform_sessions`, `platform_audit_logs`, and platform migration runs. Platform Admin authentication and tenant provisioning metadata use this database.
- **Shared tenant data plane (`DB_DSN`)**: tenant operational tables for tenants using `SHARED` mode. Its `organizations` rows are local identity rows required by tenant-schema foreign keys; they are not authoritative for platform lifecycle or database assignment.
- **Dedicated tenant databases**: configured per tenant in the platform registry, with encrypted DSNs and isolated operational tables.

Never point `PLATFORM_DB_DSN` at the same schema as `DB_DSN` in production. The application enforces this. A shared MySQL server is fine; the schemas must be different. Use separate database users and least-privilege grants in production where practical.

## First-time setup

1. Create the two MySQL schemas, for example `gopass_platform` and `gopass_tenants`, and create database users with the required privileges.
2. Set `PLATFORM_DB_DSN` and `DB_DSN` in `backend/.env`.
3. Apply control-plane migrations: `cd backend` then `go run ./cmd/platform-migrate up`. This includes the organization ID sequence, which reserves IDs above those already used in the shared tenant schema.
4. Apply shared tenant schema migrations: `go run ./cmd/migrate up`.
5. Create the first platform administrator with `PLATFORM_ADMIN_EMAIL`, `PLATFORM_ADMIN_PASSWORD`, `PLATFORM_ADMIN_NAME`, then `go run ./cmd/platform-admin-create`.
6. Start the API. New shared tenants receive a local `organizations` identity row before the platform registry marks them `READY`/`ACTIVE`.

## Existing single-database installations

1. Schedule a maintenance window and take verified backups of the existing database.
2. Create the new platform schema and configure `PLATFORM_DB_DSN` to point at it. Keep `DB_DSN` pointing at the existing database, which remains the shared tenant data plane. Preserve the existing `JWT_SECRET` and `PLATFORM_DB_ENCRYPTION_KEY` values so current sessions and encrypted dedicated DSNs remain valid.
3. Run `go run ./cmd/platform-migrate up` against the new platform schema.
4. Run `go run ./cmd/platform-import`. It copies platform registry/admin/session/audit/run rows from `DB_DSN` to `PLATFORM_DB_DSN`, only if the target platform tables are empty. It does not modify the source. If the target contains data, the command stops rather than overwriting it.
5. Only after the import succeeds, run `go run ./cmd/migrate up` against `DB_DSN`. Migration `0011_remove_legacy_platform_tables.sql` removes the old platform-only tables from the tenant data plane. Do not run this migration before importing legacy platform data.
6. Compare source and target row counts, verify a Platform Admin login, verify tenant logins, and verify tenant routing before switching live traffic.
7. Keep the source backup until the new installation has been verified. The old platform-only tables are removed by migration 0011 only after the import step.

The import is intentionally explicit; the app will not silently migrate or copy production identity data during startup. Migration 0011 removes the legacy platform-only tables after the import. Tenant `organizations` rows remain in the shared schema because they support tenant-owned foreign keys.

## Organization ID collision recovery

Platform organization IDs are also used as `organization_id` values in the shared tenant schema. Migration `0002_platform_organization_id_sequence.sql` serializes ID allocation and reserves IDs above the maximum already present in both schemas. Apply it before creating more tenants.

If a tenant created before this migration failed with `tenant identity row <id> belongs to a different slug`, open that tenant's detail page and choose **Retry**. For this specific bootstrap conflict, the API safely reassigns the failed platform registry row to a new, unused ID in a transaction, preserves its bootstrap request/password hash, records an audit event, and queues bootstrap again. It does not rename or delete the pre-existing tenant identity row. Confirm the new ID in the tenant list and wait for provisioning to reach `READY`.

## Tenant lifecycle and database modes

- `SHARED`: the platform registry points the tenant at `DB_DSN`. The provisioner validates the shared schema and creates/synchronizes the tenant-local organization identity row.
- `DEDICATED`: the platform registry stores an authenticated-encrypted DSN. Provisioning checks connectivity, rejects the platform/shared schema and duplicate assignments, applies the tenant migration set, validates required tables/columns/indexes, copies initial identity/role grants, and only then marks the tenant active.
- A populated shared tenant cannot be switched to dedicated automatically. Export/migrate its operational records as a reviewed data migration before changing its database mode.
- Suspension and provisioning state are authoritative in the platform registry. Tenant routing fails closed unless the registry says the tenant is active and ready.

## Migrations

- Platform control plane: `go run ./cmd/platform-migrate status|up`; the authenticated Platform Admin migration UI also targets **only** the platform database.
- Shared tenant schema: `go run ./cmd/migrate status|up` targets `DB_DSN`.
- Dedicated tenant schemas use the embedded tenant migration set during provisioning. Tenant migration rollout/monitoring should be treated separately from platform migrations.
- MySQL DDL can partially commit. Back up first, inspect status/logs after a failure, and retry only after correcting the cause.

## Security and operations

- Keep `PLATFORM_DB_ENCRYPTION_KEY` stable and secret; losing it makes stored dedicated DSNs unreadable.
- Never return database DSNs or credentials in API responses/logs.
- Use separate DB accounts and least-privilege grants. The account used for provisioning may require broader privileges than the normal application account; do not use that provisioning account for tenant request queries.
- Back up platform and tenant databases independently and test restores. Platform registry loss affects routing for all tenants.
- Do not use a tenant-provided database name/DSN to select a connection. Database assignment is resolved from the trusted platform registry.
