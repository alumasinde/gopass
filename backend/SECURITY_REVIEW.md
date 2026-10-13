# GoPass Security and Tenant-Isolation Review

## Changes in this hardening pass

### Revocable login sessions

- Access and refresh JWTs now share a cryptographically random `sid` per login session.
- Refresh-token rows persist that session identifier.
- The bearer middleware checks that the user is active and that the session still has an unrevoked, unexpired refresh token. Revoking a session therefore invalidates its already-issued access tokens on the next request.
- Refresh rotation keeps the same session ID and remains transactional/single-use.
- Logout validates the submitted refresh token against the active token row and revokes the whole session in a transaction.
- Migration `0007_revocable_sessions.sql` adds the session identifier and index. Existing JWTs without `sid` are rejected; clients must log in again after applying this migration.
- Disabled organizations are rejected at login and during access-token validation.

### RBAC scope enforcement

- Generic `Can` / `Require` checks now grant only `ORGANIZATION`-scoped permissions. A `SITE` or `GATE` assignment can no longer accidentally satisfy a global permission check.
- `CanWithScope` validates that referenced sites/gates exist, are active, and belong to the current organization. A nonexistent or cross-organization gate no longer falls through to a permissive fallback.
- Role permission queries require the role and assignment to belong to the same organization.
- Role assignment validates scope/resource combinations. A missing site/gate ID or mixed site+gate scope is rejected instead of being converted to organization scope.
- Role assignment SQL verifies that both target user and role belong to the current organization.
- Gatepass listing uses the user's assigned gate/site scopes. Gatepass read and workflow mutations, check-in/check-out creation, credential issue/verify/revoke, and approval actions use resource-aware scope checks.
- Endpoints that do not yet implement scope-filtered listing or resource checks intentionally require an organization-wide assignment. This is fail-closed: scoped users are denied there rather than being given organization-wide access. Add scope-filtered read paths before expanding those permissions.

### Cross-organization safeguards

- Permission evaluation joins `roles` back to `user_roles.organization_id`.
- Credential, gatepass, visitor, and approval joins now include organization consistency conditions where applicable.
- Role assignment rejects cross-organization user/site/gate references.
- User activity checks also require the parent organization to remain active.

### Audit coverage

- Added audit events for role permission assignment/removal and user-role assignment/removal, including relevant IDs and scope details.
- Login, refresh, and logout events include organization and actor information where available.
- Existing create/update/archive, gatepass workflow, approval, credential, check-in/check-out, and visitor blacklist actions continue to emit audit events.

## Migration and validation

1. Back up the database.
2. Apply migrations with the project's normal migration command; do not edit migrations `0001`–`0006`.
3. Existing clients must sign in again after migration `0007` because old JWTs do not carry a session ID.
4. Run:

```powershell
go test ./...
go test -race ./...
go vet ./...
```

5. Add/run integration tests against a real MySQL test database for cross-organization IDs, role scope combinations, logout invalidating an existing access token, refresh-vs-logout races, and audit row creation.

## Important validation note

The code in this pass was formatted with `gofmt`. Automated Go tests could not be executed in the editing environment because Go dependencies were not cached and outbound access to `proxy.golang.org` was unavailable. Run the commands above in the development environment before merging or deploying.

This pass tightens the backend, but a production approval should still include MySQL integration tests and an endpoint-by-endpoint authorization review. Audit writes currently log a server error if the database insert fails; they are not transactionally coupled to the business write.


## Scope enforcement and dedicated tenant routing (current hardening pass)

Resource-scoped authorization supports organization, site, and gate scopes. A gate-scoped permission applies only to that active gate; a site-scoped permission applies to active gates under that site; an organization-scoped permission applies to all active resources in the organization. All resource IDs are checked against the authenticated organization before access is granted. Site/gate-dependent list endpoints must use the `AccessibleSites` / `AccessibleGates` result and must return an empty list when a scoped grant has no active resources (never fall back to organization-wide access).

Visitor directory, pass-type catalogue, user administration, role administration, organization settings, and notification queue management are intentionally organization-wide administrative resources in this API. Their `Require` checks accept organization-scope grants only; a site/gate-only role cannot use these endpoints. If a future product requirement allows site-scoped visitor or pass-type management, those tables need explicit ownership relationships or list filters based on gatepass relationships before the permissions are relaxed.

The database router resolves every tenant-owned operation from the signed organization claim placed into request context by bearer middleware. Shared tenants route to the central schema; dedicated tenants route to an encrypted, validated DSN only while the central tenant lifecycle is ACTIVE and provisioning is READY. Dedicated provisioning applies the embedded versioned migrations, checks required tables/columns/uniqueness indexes, verifies that the target schema is not assigned to another tenant, copies the tenant identity/role grants, and activates only after validation succeeds. Configure `PLATFORM_DB_ENCRYPTION_KEY` as 32 random bytes (or base64-encoded 32 bytes). Do not log DSNs or passwords.

Integration tests requiring dedicated database provisioning must use `GOPASS_TEST_DSN` for a central database ending in `_test` and `GOPASS_DEDICATED_TEST_DSN` for a separate dedicated database also ending in `_test`. The dedicated database test is skipped unless both the DSN and encryption key are configured. Never point these variables at production.
