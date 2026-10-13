# GoPass timestamp and timezone convention

GoPass stores database timestamps as UTC. MySQL `DATETIME` columns do not carry timezone metadata, so values written with `UTC_TIMESTAMP()` are UTC wall-clock values even when a SQL client displays them without a `Z` suffix.

For example, a database value of `2026-10-13 14:18:13` represents `14:18:13 UTC`, which is `17:18:13` in Kenya (`Africa/Nairobi`, UTC+03:00). Do not add three hours to stored timestamps or rewrite existing session rows.

The Go MySQL driver must parse `DATETIME` values using `time.UTC`. Otherwise it interprets a UTC wall-clock value in the machine's local timezone and Go JSON serialization can emit a misleading `+03:00` offset. The main database opener, environment DSN builder, dedicated-tenant connections, tenant DSN validation and migration runner now explicitly configure `ParseTime=true` and `Loc=time.UTC`.

API timestamps should be RFC3339 UTC (with `Z`) and the frontend should convert them to `Africa/Nairobi` only for display. `frontend/src/lib/dateUtils.ts` already follows this convention.

## Verifying after restart

1. Restart the Go API after replacing the source.
2. Log in to Platform Admin again and inspect a timestamp-returning endpoint.
3. API timestamps originating from database `DATETIME` columns should end in `Z` (or use an explicit `+00:00` offset).
4. The frontend should display the same instant in Kenyan time.
5. Existing rows remain unchanged; they are already stored in UTC.

To view a raw UTC timestamp in Kenyan time in a read-only query, use `DATE_ADD(created_at, INTERVAL 3 HOUR) AS created_at_eat`. Do not use that transformed value for comparisons, writes or expiry decisions.


The MySQL driver is configured with both `Loc=UTC`/`parseTime=true` and the MySQL session system variable `time_zone='+00:00'`. `Loc` controls Go-side parsing; the session variable ensures MySQL `TIMESTAMP` defaults and conversions use UTC too. The API, CLI migrator, and dedicated-tenant connections must preserve both settings.
