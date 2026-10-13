# Frontend module organization

The frontend is organized by product domain under `src/modules/`:

- `auth/`: tenant and platform authentication pages.
- `dashboard/`: tenant and platform dashboards.
- `admin/`: platform tenant/admin/migration screens.
- `visitors/`: visitor directory, visitor details, registration, edits and blacklist actions.
- `visits/`: scheduled access/visit workflow UI.
- `gatepass/`: gatepass domain API exports used by visit scheduling and future gatepass screens.

Routes import modules directly. `src/pages/` currently contains compatibility re-exports only; new page implementations belong under `src/modules/<domain>/`.

## Styles

`src/main.tsx` imports `src/index.css` as the single application stylesheet entry point. Feature styles live in `src/styles/` and are imported by `index.css`; legacy static inline styles have also been extracted into centrally defined classes. Views should use class names and must not import CSS files or add inline styling.

## Backend contract

Visitor UI calls the existing `/api/v1/visitors/` list/create routes, `/api/v1/visitors/{id}` get/update routes, and blacklist/unblacklist actions. Visit scheduling currently uses the backend's gatepass resource because this source does not define a dedicated `/api/v1/visits` endpoint. The Visits UI creates draft gatepasses and uses existing submit/cancel/revoke actions. A dedicated Visit entity and API can be added later if visit scheduling needs its own lifecycle separate from gatepasses.
