# GoPass Tenant Authentication & Dashboard

This change adds tenant-facing authentication and a dashboard to the existing React frontend. Platform Admin routes remain separate.

## Routes

- `/tenant/login` — tenant sign-in using organization slug, email and password.
- `/tenant/dashboard` — authenticated tenant dashboard.
- `/tenant` — redirects to the tenant dashboard or login depending on session state.
- Platform Admin remains available under `/platform/*`.

## API integration

The tenant UI uses a dedicated Axios client and separate local session keys (`tenant_access_token`, `tenant_refresh_token`). It does not send `platform_access_token` to tenant endpoints.

- `POST /api/v1/auth/login` with `organization_slug`, `email`, `password`
- `GET /api/v1/auth/me`
- `POST /api/v1/auth/logout` with `refresh_token`
- `GET /api/v1/organization`
- `GET /api/v1/visitors/`
- `GET /api/v1/gatepasses/`
- `GET /api/v1/approvals/`
- `GET /api/v1/check-ins/`
- `GET /api/v1/check-outs/`

The dashboard requests the existing APIs directly. It does not add a fake dashboard endpoint or hard-coded sample records. If a data endpoint is forbidden or unavailable, the relevant count is shown as unavailable and the page identifies the affected endpoint group.

## Test locally

1. Start MySQL and ensure platform and tenant migrations have been applied.
2. Start the backend from `backend` with `go run ./cmd/api`.
3. Start the frontend from `frontend` with `npm ci` and `npm run dev`.
4. Open `http://localhost:5173/tenant/login`.
5. Enter the organization slug from Platform Admin → Tenants, plus the tenant administrator email and password entered when creating that tenant.
6. Confirm the dashboard loads the organization, user identity and live visitor/gatepass/approval/check-in/check-out records.
7. Refresh the dashboard to confirm the saved session is revalidated through `/api/v1/auth/me`.
8. Sign out and confirm the dashboard redirects to tenant login. Confirm `/platform/login` continues to use Platform Admin credentials independently.

## Current scope

This phase implements tenant sign-in/session handling and a read-only dashboard. Visitors, gatepasses, approvals and security desk navigation are intentionally marked as upcoming pages; their existing backend endpoints are already used for dashboard summaries. It does not change backend authorization or introduce new endpoints.
