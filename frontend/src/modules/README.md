# Frontend module organization

The frontend is organized by product domain under `src/modules/`:

- `auth/`: tenant and platform authentication pages.
- `dashboard/`: tenant and platform dashboards.
- `admin/`: platform tenant/admin/migration screens.
- `visitors/`: visitor directory, company and identity document capture, dynamic risk scoring, blacklist actions.
- `visits/`: scheduled access/visit workflow UI.
- `gatepass/`: gatepass domain API exports used by visit scheduling and future gatepass screens.
- `sites/`, `gates/`, `pass-types/`, `identification-types/`: configurable tenant reference data and accepted identity documents.
- `users/`, `roles/`, `organization/`: tenant access control and organization settings.
- `components/crud/`: shared data table, form modal, confirmation dialog, and CRUD state hook.

Routes import modules directly. Page implementations belong under `src/modules/<domain>/`.

## Styles

`src/main.tsx` imports `src/index.css` as the single application stylesheet entry point. Feature styles live in `src/styles/` and are imported by `index.css`; shared CRUD styles are defined in `src/styles/crud.css`. Views should use class names and must not import CSS files or add inline styling.

## Backend contract

Visitor UI calls the existing `/api/v1/visitors/` list/create routes, `/api/v1/visitors/{id}` get/update routes, blacklist/unblacklist actions, and the new soft-archive endpoint. Visit scheduling currently uses the backend's gatepass resource because this source does not define a dedicated `/api/v1/visits` endpoint. The Visits UI creates and edits draft gatepasses, uses saved visitor records, displays visitor company/identity/risk, and uses existing submit/cancel/revoke actions. The backend independently checks active blacklist entries before create, update, or submit. A dedicated Visit entity and API can be added later if visit scheduling needs its own lifecycle separate from gatepasses.

Identification type defaults are lazily ensured per organization so tenants provisioned after schema migration receive the default document catalogue too. Visitor risk is computed from current data by the API and is not persisted as a stale score.
