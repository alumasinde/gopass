# Visitor identity and risk engine

## Apply the migrations

Back up the platform and tenant databases first. From `backend/`, run:

```powershell
go run ./cmd/tenant-migrate up
```

The migration sequence keeps the checksum migration at `0010_schema_migration_checksums.sql`, preserves `0011_remove_legacy_platform_tables.sql` and `0012_visitors_archive.sql`, then adds `0013_identification_types.sql` and `0014_visitor_identity_risk.sql`. This avoids duplicate migration version numbers.

The tenant migration command applies pending migrations to the shared tenant database and active, ready dedicated tenant databases. Confirm that every database reports migration 0014 as applied before using the new visitor fields.

## Identification types

`GET /api/v1/identification-types/` lists the organization's active and inactive types. Create/update/delete use the `identification_types.manage` permission. Codes are stable and uppercase (for example `NATIONAL_ID`, `PASSPORT`, `ALIEN_ID`, `REFUGEE_ID`, `DRIVING_LICENCE`, `WORK_PERMIT`, `STUDENT_ID`, `EMPLOYEE_ID`, `MILITARY_ID`, `DIPLOMATIC_ID`, `OTHER`). A type referenced by a visitor cannot be deleted; deactivate it instead. Default types cannot be deleted. An organization can choose one default type.

Validation regex is optional and evaluated by Go's RE2-compatible regular-expression engine. It validates format only and does not authenticate a document or verify that it belongs to the visitor.

## Visitor fields

Visitor create/update now accepts `company_name`, `id_type_id`, `id_number`, and `id_expiry_date` (`YYYY-MM-DD`) in addition to name, phone and email. An ID type and ID number must be supplied together. Identity numbers are normalized to uppercase and duplicate active identity numbers within the same organization and type are rejected.

## Risk calculation

Risk is computed from the current visitor record on list/get; it is not a stored value that can go stale.

- Missing phone: +20
- Missing email: +15
- Missing identification type or number: +30
- Expired identification document: +15
- Missing company/affiliation: +5
- A matching active blacklist record: score 100, `CRITICAL`

Bands: `LOW` 0-24, `MEDIUM` 25-49, `HIGH` 50-74, `CRITICAL` 75-100. The API returns `risk_score`, `risk_level`, and `risk_factors`.

Missing contact information increases risk but does not by itself add a visitor to the blacklist. This prevents missing data from becoming an automatic permanent denial. A confirmed active blacklist entry is checked again by the gatepass create/update/submit workflows, so a UI bypass cannot issue access to a blacklisted visitor. Expired blacklist entries are ignored.

## Test

- Create a visitor with full contact and identity details and confirm a low score.
- Remove phone/email and verify the score/factors change on the next API read.
- Enter an expired ID date and confirm the expired-document factor.
- Blacklist a visitor, then verify the score becomes 100/CRITICAL and gatepass create/submit is rejected.
- Unblacklist a visitor and confirm the visitor-linked blacklist entry is inactive.
- Try duplicate ID numbers of the same type in one organization; expect HTTP 409.
- Confirm another organization cannot read or use the visitor or identification type.
