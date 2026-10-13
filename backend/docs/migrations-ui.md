# Database migrations UI

The Platform Admin **Database migrations** page reads the migration inventory embedded in the running Go backend and compares it with `schema_migrations` in the configured central platform database. It does not use mock rows and it does not edit SQL files.

## Endpoints

- `GET /api/v1/platform/migrations` — actual applied/pending status, target database name, applied timestamps, checksum status, and drift detection. Requires a valid platform administrator session.
- `POST /api/v1/platform/migrations/run` — queue pending embedded migrations. Requires a valid platform administrator session, `PLATFORM_MIGRATIONS_UI_ENABLED=true`, exact confirmation phrase `APPLY PENDING MIGRATIONS`, and the pending version list returned by the latest status request.
- `GET /api/v1/platform/migrations/runs/{id}` — persisted execution state and applied versions for a migration run.

The runner and CLI share the MySQL advisory lock `gopass_schema_migrate`, preventing simultaneous CLI/UI migration execution. Migration queue/start/success/failure events are recorded in `platform_audit_logs` in the same transaction as each run-state transition. The job status is persisted in `platform_migration_runs`; the API returns quickly and the frontend polls status, so the HTTP server's normal request write timeout does not truncate a long migration.

## Enable only during a maintenance window

Keep this in the backend `.env` by default:

```dotenv
PLATFORM_MIGRATIONS_UI_ENABLED=false
```

After a backup and review of pending migrations, set it to `true` and restart the backend. Run migrations from the UI, verify the final status, then set it back to `false` and restart the backend.

This endpoint only migrates the **central database selected by the backend's `DB_DSN`**. It does not run tenant-specific migrations against dedicated tenant databases. Dedicated tenant database migrations must continue to use the tenant provisioning workflow.

For a brand-new database, use the CLI bootstrap path first so the platform administrator and platform routes exist. The UI is for safely operating migrations on an already bootstrapped control plane.

## Checksums

Migration `0010_schema_migration_checksums.sql` adds a nullable SHA-256 checksum column to `schema_migrations` and creates the migration-run tracking table. Existing applied rows remain NULL because older releases did not record checksums. New migrations applied after 0010 record checksums. The UI/runner blocks execution if a recorded checksum or migration name differs from the source, or if the database records a migration version absent from the current build.

## Operational safety

MySQL DDL auto-commits and cannot be rolled back as a whole transaction. Back up first. If a migration fails, inspect backend logs and migration status before retrying; do not edit an already-applied migration. Add a new migration to correct schema state. A failed migration can have partially applied DDL; the runner leaves the run record failed, clears the active-run key, and relies on the migration's rerunnable design for a deliberate retry.
