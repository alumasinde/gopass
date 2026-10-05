# PassNow — Phase 1

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
1. Create MySQL database and apply `migrations/0001_foundation.sql`, then `0002_permissions.sql`.
2. Copy `.env.example` to `.env` and set a strong `JWT_SECRET`.
3. Run `go mod tidy`.
4. Run `go test ./...`.
5. Run `go run ./cmd/api`.

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
