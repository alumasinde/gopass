# Visitors and Visits UI smoke test

Run the backend and frontend with a tenant whose provisioning status is `READY`.

## Required permission codes

- Visitors list/profile: `visitors.view`
- Register visitor: `visitors.register`
- Edit visitor: `visitors.update`
- Blacklist/unblacklist: `visitors.blacklist`
- View visits: `gatepasses.view`
- Schedule a visit: `gatepasses.create`, `visitors.view`, `gates.view`, `pass_types.view`
- Submit a visit draft: `gatepasses.submit`
- Cancel a visit: `gatepasses.cancel`
- Revoke approved/issued access: `gatepasses.revoke`

The UI uses these permissions for navigation and actions. The API remains the authority and may still return `403` for gate/site scope restrictions.

## Visitors

1. Open `/tenant/visitors`; confirm the list comes from `GET /api/v1/visitors/`.
2. Register a visitor; confirm `POST /api/v1/visitors/` and verify the record appears after refresh.
3. Edit a visitor; confirm `PUT /api/v1/visitors/{id}`.
4. Blacklist and unblacklist a visitor; confirm the corresponding POST action and status update.
5. Search by name, phone, email and ID; switch between all, active and blacklisted filters.
6. Sign in with a role lacking `visitors.view`; confirm the navigation item is hidden and direct route access shows a permission message.

## Visits

1. Open `/tenant/visits`; confirm records load from `GET /api/v1/gatepasses/`.
2. Schedule a visit using an existing non-blacklisted visitor, active gate and active pass type. Confirm the request creates a `DRAFT` gatepass through `POST /api/v1/gatepasses/`.
3. Submit the draft. Confirm `POST /api/v1/gatepasses/{id}/submit` changes the state to `APPROVED` or `PENDING_APPROVAL` according to the configured pass type and workflow.
4. Verify cancellation is offered only for `DRAFT`, `PENDING_APPROVAL` and `APPROVED` records; revoke is offered only for `APPROVED` and `ISSUED` records.
5. Verify invalid times, past end times, missing selections and backend `403`/`409` responses are shown as errors.
6. Sign in with a role lacking `gatepasses.view`; confirm the Visits navigation item is hidden and direct route access is denied.

## API contract note

This backend currently has no dedicated `/api/v1/visits` resource. The Visits UI therefore presents scheduled visitor access using existing gatepass records and endpoints; it does not call an invented visits endpoint.
