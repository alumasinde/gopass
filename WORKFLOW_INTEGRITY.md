# Workflow and database integrity hardening

## Runtime behavior

- Gatepass creation locks and validates the visitor, gate, and pass type in one transaction.
- Gatepass submit/cancel/revoke, approval decisions, credential issue/revoke, check-in, and check-out use row locks and conditional status updates.
- Critical workflow state changes write audit rows through the same SQL transaction. An audit insert failure rolls back the workflow mutation.
- Approval decisions lock the gatepass row as well as the approval request so simultaneous decisions serialize.
- Check-in and check-out are protected by row locks plus unique database constraints.

## Migration 0008

`0008_workflow_integrity.sql` is additive. It does not edit migrations `0001`–`0007`. It adds `gatepasses.created_by`, tenant-consistent composite foreign keys, and database uniqueness constraints for:
- one check-in per gatepass;
- one check-out per gatepass;
- one active credential per gatepass;
- one pending approval request per gatepass;
- one request per workflow step.

The migration uses generated nullable columns for the two conditional uniqueness rules; MySQL unique indexes permit multiple `NULL` values but reject duplicate non-NULL active keys.

## Apply safely

Back up the database before applying the migration. It deliberately fails if existing rows violate the new constraints; it does not silently delete or rewrite business records. Run the duplicate/cross-tenant checks below first if the migration fails.

```sql
SELECT organization_id, gatepass_id, COUNT(*) AS n
FROM check_ins GROUP BY organization_id, gatepass_id HAVING COUNT(*) > 1;

SELECT organization_id, gatepass_id, COUNT(*) AS n
FROM check_outs GROUP BY organization_id, gatepass_id HAVING COUNT(*) > 1;

SELECT organization_id, gatepass_id, COUNT(*) AS n
FROM credentials WHERE is_revoked = 0
GROUP BY organization_id, gatepass_id HAVING COUNT(*) > 1;

SELECT organization_id, gatepass_id, COUNT(*) AS n
FROM approval_requests WHERE status = 'PENDING'
GROUP BY organization_id, gatepass_id HAVING COUNT(*) > 1;

SELECT gp.id, gp.organization_id, gp.visitor_id
FROM gatepasses gp JOIN visitors v ON v.id = gp.visitor_id
WHERE gp.organization_id <> v.organization_id;

SELECT gp.id, gp.organization_id, gp.gate_id
FROM gatepasses gp JOIN gates g ON g.id = gp.gate_id
WHERE gp.organization_id <> g.organization_id;

SELECT gp.id, gp.organization_id, gp.pass_type_id
FROM gatepasses gp JOIN pass_types pt ON pt.id = gp.pass_type_id
WHERE gp.organization_id <> pt.organization_id;
```

Review and repair inconsistent records deliberately before rerunning `go run ./cmd/migrate up`. MySQL DDL may partially apply before a migration error; the migration is guarded by `information_schema` checks so rerunning after correcting the cause completes the remaining statements.

## Verification

```powershell
go test ./...
go test -tags=integration -v ./internal/security/integration
go test -tags=integration ./...
go vet -tags=integration ./...
```

The integration tests require `GOPASS_TEST_DSN` pointing to a dedicated database whose name ends in `_test`. Do not run destructive tests against production.
