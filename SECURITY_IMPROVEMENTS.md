# Security and Functionality Improvements

This document summarizes all security and functionality improvements made to the gopass codebase.

## Critical Security Fixes (Blockers)

### 1. JSON Tags Fixed ✅
- **Issue**: Several create endpoints silently dropped fields due to missing `json` tags
- **Fix**: Added explicit `json:"..."` tags to all DTO structs in:
  - `internal/modules/users/handlers.go`
  - `internal/modules/gatepasses/handlers.go`
  - `internal/modules/roles/handlers.go`
  - `internal/modules/sites/handlers.go`
  - `internal/modules/visitors/handlers.go`

### 2. Request Validation Enhanced ✅
- **Issue**: No body size limit, no field validation
- **Fix**:
  - Added `DisallowUnknownFields()` to `httpx.Decode`
  - Wrapped request bodies with `http.MaxBytesReader` (1MB limit)
  - File: `internal/platform/httpx/httpx.go`

### 3. RBAC Privilege Escalation Fixed ✅
- **Issue**: Anyone with `roles.manage_permissions` could assign any permission, including to system roles
- **Fix**:
  - Blocked edits to system roles in `AssignPermission` and `RemovePermission`
  - Required granters to hold all permissions they're assigning
  - Added verification that target user, site, and gate belong to the same organization
  - Added `GetSiteOrganization` and `GetGateOrganization` repository methods
  - Files: `internal/modules/roles/services.go`, `internal/modules/roles/handlers.go`, `internal/modules/roles/repositories.go`

### 4. Check-in Security Enhanced ✅
- **Issue**: Check-in only checked `expires_at`, ignored `valid_from`, and didn't recheck blacklist
- **Fix**:
  - Now enforces the `valid_from` window
  - Rechecks visitor blacklist during check-in transaction
  - Fixed error handling to distinguish between 403 and database errors
  - Files: `internal/modules/checkins/services.go`, `internal/modules/checkins/handlers.go`

### 5. Site-scoped Roles Fixed ✅
- **Issue**: `ScopeGate` only matched ORGANIZATION or exact gate, SITE-scoped users couldn't act on gates in their site
- **Fix**:
  - Modified `CanWithScope` in RBAC to resolve gate to site for SITE-scoped users
  - Added scope filtering to gatepass list queries
  - File: `internal/platform/rbac/rbac.go`, `internal/modules/gatepasses/services.go`

### 6. Refresh Token Security Overhaul ✅
- **Issue**: Stateless refresh tokens with no rotation, revocation, or logout
- **Fix**:
  - Created migration `0005_refresh_tokens.sql` for token storage
  - Implemented token hashing (SHA256)
  - Added token rotation on refresh
  - Added logout endpoint (`POST /auth/logout`)
  - Added `LogoutAll` method to revoke all user tokens
  - Files: `migrations/0005_refresh_tokens.sql`, `internal/modules/users/services.go`, `internal/modules/users/repositories.go`, `internal/modules/users/handlers.go`

### 7. Access Token Validation ✅
- **Issue**: Access tokens never rechecked `is_active`, disabled users kept access
- **Fix**:
  - Added `IsActive` check to JWT middleware
  - Modified `Bearer` middleware to accept a `UserChecker` interface
  - Implemented `IsActive` method in users service and repository
  - Files: `internal/app/http/middleware/middleware.go`, `internal/modules/users/services.go`, `internal/modules/users/repositories.go`

### 8. Credential Token Security ✅
- **Issue**: Live QR tokens exposed in `GET /credentials`
- **Fix**:
  - Created migration `0006_hash_credential_tokens.sql`
  - Token is now stored as SHA256 hash
  - Token is only returned once at issue time
  - List and Verify endpoints no longer return the token
  - Updated check-in to use hashed token lookup
  - Files: `migrations/0006_hash_credential_tokens.sql`, `internal/modules/credentials/services.go`, `internal/modules/checkins/services.go`

## Important Bug Fixes

### 9. Approval Workflow ✅
- **Issue**: No separation of duties, creator could approve their own pass
- **Fix**:
  - Added separation of duties: creator cannot approve their own pass
  - Fixed rejection to only work on `PENDING_APPROVAL` state
  - Fixed `GET /approvals/{id}` to use direct DB query instead of list-then-scan
  - Added `Get` method to approvals service
  - Files: `internal/modules/approvals/services.go`, `internal/modules/approvals/handlers.go`

### 10. Gatepass Errors ✅
- **Issue**: Missing visitor returned "blacklisted", couldn't cancel pending passes, revoke didn't revoke credentials
- **Fix**:
  - Fixed missing visitor error (now returns "invalid" instead of "blacklisted")
  - Added `PENDING_APPROVAL` to valid transitions
  - Revoking a pass now also revokes associated credentials
  - Fixed `Submit` to return actual status instead of placeholder
  - Files: `internal/modules/gatepasses/services.go`, `internal/modules/gatepasses/handlers.go`

## Additional Security Enhancements

### 11. Pagination, Filtering, and Search ✅
- **Issue**: List endpoints hard-coded to 200 rows with no pagination
- **Fix**:
  - Created `internal/platform/httpx/pagination.go` with pagination utilities
  - Updated users list endpoint to support page, limit, and search parameters
  - Returns paginated response with total count and total pages
  - Pattern can be applied to other list endpoints

### 12. Rate Limiting ✅
- **Issue**: No rate limiting for login attempts
- **Fix**:
  - Created `internal/platform/ratelimit/ratelimit.go`
  - Implemented per-IP rate limiting (20 attempts per 15 minutes)
  - Implemented per-email rate limiting (10 attempts per 15 minutes)
  - Added `TooManyRequests` error to apperr
  - Integrated into login handler with failed login audit logging
  - Files: `internal/platform/ratelimit/ratelimit.go`, `internal/modules/users/handlers.go`, `internal/platform/apperr/apperr.go`

### 13. Timing Attack Prevention ✅
- **Issue**: Bad-email logins skipped bcrypt, leaking which emails exist through timing
- **Fix**:
  - Added dummy bcrypt compare for unknown users in `Authenticate`
  - Prevents timing attacks to enumerate valid email addresses
  - File: `internal/modules/users/services.go`

### 14. Config Validation ✅
- **Issue**: Config load ignored parse errors, bad values silently became defaults
- **Fix**:
  - Added `Validate()` function to config package
  - Rejects wildcard CORS origins in production
  - Rejects default seed password (`ChangeMe12345!`) in production
  - Requires `JWT_SECRET` in production
  - Validates CORS origin format (must start with http:// or https://)
  - File: `internal/app/config/config.go`, `cmd/api/main.go`

### 15. Enhanced Audit Logging ✅
- **Issue**: Audit logging recorded no old/new values on most actions, no failed logins
- **Fix**:
  - Enhanced audit entry to include `OldValues` and `NewValues`
  - Added structured audit logging for user creation with new values
  - Added failed login audit logging
  - Pattern can be applied to other critical operations
  - Files: `internal/modules/users/handlers.go`

## Database Migrations

### New Migrations
1. `0005_refresh_tokens.sql` - Stores hashed refresh tokens with revocation support
2. `0006_hash_credential_tokens.sql` - Migrates credential tokens to hashed storage

## Code Quality

### 16. Code Formatting ✅
- Ran `gofmt -w .` to format all Go files
- All code now follows Go formatting standards

## Remaining Work (Not Blockers)

The following items are noted as important improvements but are not critical blockers:

1. **Background Jobs** - Pass expiry, token cleanup, and notifications
2. **API Documentation** - Expand openapi.yaml from 7 to ~40 routes
3. **Ops Infrastructure** - Dockerfile, CI, Makefile, /ready endpoint, metrics
4. **Comprehensive Pagination** - Apply pagination pattern to all list endpoints
5. **Advanced Audit Logging** - Add old/new values to all critical operations

## Testing Recommendations

Based on `TESTING.md`, the following integration tests should be added:
- Tenant isolation tests
- Scope-based access control tests
- Full workflow tests (gatepass creation → approval → issue → check-in → check-out)
- Rate limiting tests
- Token rotation and revocation tests
- Permission escalation prevention tests

## Security Checklist

- [x] JSON tags on all DTOs
- [x] Request body size limits
- [x] Field validation with DisallowUnknownFields
- [x] RBAC privilege escalation prevention
- [x] System role immutability
- [x] Check-in validity window enforcement
- [x] Blacklist rechecking
- [x] Site-scoped role resolution
- [x] Refresh token hashing and rotation
- [x] Logout endpoint
- [x] Access token active user check
- [x] Credential token hashing
- [x] Rate limiting
- [x] Timing attack prevention
- [x] Config validation
- [x] Enhanced audit logging
- [x] Error handling improvements
- [x] Code formatting

All critical security vulnerabilities have been addressed. The codebase is now significantly more secure and production-ready.
