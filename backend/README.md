# GoPass — Phase 1

Production-oriented Go/Chi foundation for a multi-tenant gatepass platform.

## Stack
- Go + Chi
- MySQL
- slog structured logs
- JWT access/refresh authentication
- RBAC with organization/site/gate scope fields
- Database-backed audit logs
- No SQL ENUMs
- Business configuration belongs in MySQL; infrastructure configuration belongs in `.env`.

## API
All application endpoints are under `/api/v1`. OpenAPI is in `docs/openapi.yaml`.

## Run
1. Copy `.env.example` to `.env`, set `DB_DSN` (or the `DB_*` parts) and a strong `JWT_SECRET`.
2. Apply the schema: `go run ./cmd/migrate up` (creates the database if missing, applies every pending file in `migrations/`; check with `go run ./cmd/migrate status`).
3. Optional demo data: `go run ./cmd/seed`.
4. Run `go mod tidy`, then `go test ./...`.
5. Run `go run ./cmd/api`.

Migration files must not contain `CREATE DATABASE` / `USE`; the target database comes from `.env`. Write them re-runnable (`IF NOT EXISTS`, guarded `ALTER`s, `INSERT IGNORE`) because MySQL DDL cannot be rolled back.

## Security design
Permissions are domain/action capabilities (`gatepasses.approve`, `checkins.perform`, etc.), not role checks. Roles grant permissions; user-role assignments carry a scope (`ORGANIZATION`, `SITE`, or `GATE`). Tenant-owned queries always include `organization_id`.

## Phase 1 modules
Auth/users, organizations, roles, sites, gates, visitors, gatepasses, approvals, credentials, check-ins, check-outs, RBAC, tenancy, audit and configuration foundation.

## Production notes
- Restrict `CORS_ALLOWED_ORIGINS` to known frontend origins; never use `*` with credentials.
- Put TLS termination at the reverse proxy/load balancer.
- Rotate JWT secrets through deployment secrets management.
- Use a dedicated MySQL account with only required privileges.
- Add Redis/Celery-equivalent Go workers in Phase 2 for notifications and scheduled expiry processing.

## Module convention

Each business module owns its transport and domain boundaries:

```text
module/
├── dto.go
├── models.go
├── repositories.go
├── services.go
├── handlers.go
├── routes.go
├── policies.go
└── errors.go
```

`internal/routes/api.go` is the API v1 composition point. `cmd/api/main.go` only loads configuration, constructs the application, and runs it. There are no `module.go` files.

## Platform administration and notifications

See [`PLATFORM_AND_NOTIFICATIONS.md`](PLATFORM_AND_NOTIFICATIONS.md) for migration `0009`, platform-admin bootstrap, tenant lifecycle/provisioning endpoints, tenant-aware dedicated-DB routing and migration provisioning, notification delivery/retries, and verification commands.


### Tenant database modes

The backend supports `SHARED` tenants in the central MySQL schema and `DEDICATED` tenants through the tenant-aware database router. Dedicated mode requires `PLATFORM_DB_ENCRYPTION_KEY` (32 random bytes or base64 of 32 bytes); configure the dedicated database through the platform API and allow the provisioning worker to apply the embedded migrations and validate the schema before the tenant is marked READY/ACTIVE. Failed provisioning remains inactive and can be retried through the platform administration endpoint.

### Security integration checks

Run standard tests with `go test ./...`, integration tests with `go test -tags=integration -v ./internal/security/integration`, and static analysis with `go vet -tags=integration ./...`. Use only isolated MySQL test databases ending in `_test`. To exercise dedicated database routing, set `GOPASS_DEDICATED_TEST_DSN` to a separate empty `_test` database and configure `PLATFORM_DB_ENCRYPTION_KEY`.


**Shared-to-dedicated migration safety:** current automated provisioning copies tenant identity and role grants, but does not migrate existing visitor, site, gatepass, check-in/out, workflow, audit, notification, or organization-setting history. Therefore the API and provisioner refuse to switch a shared tenant with operational records to dedicated mode. Export/migrate that history with a separately reviewed data migration before changing database mode; do not bypass this guard. New tenants with only their initial user/role grants can be provisioned directly into a dedicated database.


## Platform Admin database migrations UI

See [docs/migrations-ui.md](docs/migrations-ui.md). The UI reads actual embedded migration files and database history; execution is disabled by default and must be enabled temporarily with `PLATFORM_MIGRATIONS_UI_ENABLED=true` during an approved maintenance window. For a new database, bootstrap through `go run ./cmd/migrate up` before using the UI. Migration 0010 adds checksum history and persistent UI-run tracking.
