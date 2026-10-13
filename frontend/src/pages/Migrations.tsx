import { useCallback, useEffect, useMemo, useState } from 'react';
import { apiErrorMessage } from '../lib/api';
import { formatLocalDate } from '../lib/dateUtils';
import { platformMigrations, type MigrationStatusResponse } from '../lib/platform-api';
import './Migrations.css';

const CONFIRMATION = 'APPLY PENDING MIGRATIONS';

export default function Migrations() {
  const [status, setStatus] = useState<MigrationStatusResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [running, setRunning] = useState(false);
  const [showConfirm, setShowConfirm] = useState(false);
  const [confirmation, setConfirmation] = useState('');
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');

  const refresh = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setStatus(await platformMigrations.status());
    } catch (err) {
      setError(apiErrorMessage(err, 'Unable to load migration status. Check the backend and your platform-admin session.'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (!status?.active_run) return;
    const timer = window.setInterval(() => { void refresh(); }, 2000);
    return () => window.clearInterval(timer);
  }, [status?.active_run?.run_id, status?.active_run?.status, refresh]);

  const report = status?.report;
  const pendingVersions = useMemo(
    () => report?.migrations.filter((item) => !item.applied).map((item) => item.version) ?? [],
    [report],
  );
  const hasDrift = (report?.drift_count ?? 0) > 0;
  const hasUnverified = report?.migrations.some((item) => item.applied && item.checksum_status === 'UNVERIFIED') ?? false;
  const activeRunStale = Boolean(status?.active_run && Date.now() - new Date(status.active_run.updated_at).getTime() > 15 * 60 * 1000);
  const canRun = Boolean(status?.execution_enabled && report?.history_exists && (report?.applied_count ?? 0) > 0 && pendingVersions.length > 0 && !hasDrift && (!status?.active_run || activeRunStale) && !running);

  const runMigrations = async () => {
    if (confirmation !== CONFIRMATION || !canRun) return;
    setRunning(true);
    setError('');
    setSuccess('');
    try {
      const queued = await platformMigrations.run(pendingVersions, confirmation);
      setShowConfirm(false);
      setConfirmation('');
      setSuccess(`Migration run ${queued.run_id} queued. Waiting for the backend to finish…`);
      const deadline = Date.now() + 10 * 60 * 1000;
      while (Date.now() < deadline) {
        await new Promise((resolve) => window.setTimeout(resolve, 1500));
        const run = await platformMigrations.getRun(queued.run_id);
        if (run.status === 'SUCCEEDED') {
          setSuccess(`Migration run completed. Applied versions: ${run.applied_versions.map((v) => String(v).padStart(4, '0')).join(', ') || 'none'}.`);
          await refresh();
          return;
        }
        if (run.status === 'FAILED') {
          throw new Error(run.error_message || 'Migration run failed. Refresh status and inspect backend logs.');
        }
        setSuccess(`Migration run ${run.status.toLowerCase()} (${queued.run_id})…`);
      }
      setSuccess(`Migration run ${queued.run_id} is still active. You can refresh status; do not start another run.`);
      await refresh();
    } catch (err) {
      const message = apiErrorMessage(err, '') || (err instanceof Error ? err.message : 'Migration execution failed. Refresh status and inspect backend logs before retrying.');
      await refresh();
      setSuccess('');
      setError(message);
    } finally {
      setRunning(false);
    }
  };

  return (
    <div className="migrations-page">
      <header className="migrations-header">
        <div>
          <div className="eyebrow">PLATFORM OPERATIONS</div>
          <h1>Database migrations</h1>
          <p>Inspect the real schema history and apply pending migrations for the central platform database.</p>
        </div>
        <button className="button button-secondary" onClick={() => void refresh()} disabled={loading || running}>
          <span aria-hidden="true">↻</span> Refresh status
        </button>
      </header>

      {error && <div className="notice notice-error" role="alert"><strong>Action required</strong><span>{error}</span></div>}
      {success && <div className="notice notice-success" role="status"><strong>Migration operation finished</strong><span>{success}</span></div>}

      {loading && !status ? (
        <div className="migrations-loading" role="status">Loading migration history from the backend…</div>
      ) : !status ? (
        <div className="migrations-empty">
          <h2>Migration status unavailable</h2>
          <p>The UI could not read the backend migration history. No migration was run.</p>
          <button className="button button-primary" onClick={() => void refresh()}>Try again</button>
        </div>
      ) : (
        <>
          <section className="migration-meta">
            <div>
              <span className="meta-label">Target database</span>
              <strong>{status.target_database || 'Unknown database'}</strong>
            </div>
            <div>
              <span className="meta-label">Execution from UI</span>
              <strong className={status.execution_enabled ? 'text-success' : 'text-warning'}>
                {status.execution_enabled ? 'Enabled by server configuration' : 'Disabled by server configuration'}
              </strong>
            </div>
            <div>
              <span className="meta-label">Migration source</span>
              <strong>Embedded in this backend build</strong>
            </div>
          </section>

          {status.latest_run && (
            <div className={`notice ${status.latest_run.status === 'FAILED' ? 'notice-error' : status.latest_run.status === 'SUCCEEDED' ? 'notice-success' : 'notice-warning'}`}>
              <strong>Latest migration run: {status.latest_run.status}</strong>
              <span>
                Run {status.latest_run.run_id} · target {status.latest_run.target_database} · requested versions {status.latest_run.expected_versions.map((v) => String(v).padStart(4, '0')).join(', ')}.
                {status.latest_run.applied_versions.length > 0 ? ` Applied: ${status.latest_run.applied_versions.map((v) => String(v).padStart(4, '0')).join(', ')}.` : ''}
                {status.latest_run.error_message ? ` Error: ${status.latest_run.error_message}` : ''}
              </span>
            </div>
          )}

          <section className="migration-stats" aria-label="Migration summary">
            <article className="migration-stat">
              <span>Total known</span>
              <strong>{report?.migrations.length ?? 0}</strong>
              <small>Versioned SQL files in this build</small>
            </article>
            <article className="migration-stat">
              <span>Applied</span>
              <strong className="text-success">{report?.applied_count ?? 0}</strong>
              <small>Recorded in schema_migrations</small>
            </article>
            <article className="migration-stat">
              <span>Pending</span>
              <strong className={(report?.pending_count ?? 0) > 0 ? 'text-warning' : ''}>{report?.pending_count ?? 0}</strong>
              <small>Not yet recorded as applied</small>
            </article>
            <article className="migration-stat">
              <span>History drift</span>
              <strong className={hasDrift ? 'text-danger' : 'text-success'}>{report?.drift_count ?? 0}</strong>
              <small>Missing source, name or checksum mismatch</small>
            </article>
          </section>

          {status.active_run && (
            <div className="notice notice-warning" role="status">
              <strong>{activeRunStale ? 'A stale migration run needs recovery' : `A migration run is ${status.active_run.status.toLowerCase()}`}</strong>
              <span>Run {status.active_run.run_id} targets {status.active_run.target_database} for versions {status.active_run.expected_versions.map((v) => String(v).padStart(4, '0')).join(', ')}. {activeRunStale ? 'It has been inactive for over 15 minutes; the next confirmed run will mark it failed and retry only versions still pending.' : 'This page refreshes status automatically. Do not start another run.'}</span>
            </div>
          )}

          {report?.history_exists && report.applied_count === 0 && report.pending_count > 0 && (
            <div className="notice notice-warning">
              <strong>No applied migration history exists</strong>
              <span>The UI will not bootstrap an untracked schema. Use the backend migration CLI to bootstrap or repair the database, then refresh this page.</span>
            </div>
          )}

          {!report?.history_exists && (
            <div className="notice notice-warning">
              <strong>Migration history table not found</strong>
              <span>The database has no schema_migrations table. The UI will not execute migrations against an untracked schema. Bootstrap or repair the database with the migration CLI first, then refresh this page.</span>
            </div>
          )}

          {!status.execution_enabled && (
            <div className="notice notice-warning">
              <strong>Execution is disabled</strong>
              <span>For a controlled maintenance window, set <code>PLATFORM_MIGRATIONS_UI_ENABLED=true</code> on the backend and restart it. Keep this disabled by default in production and enable it only when an authorized operator needs to run migrations.</span>
            </div>
          )}

          {hasDrift && (
            <div className="notice notice-error">
              <strong>Migration history drift detected</strong>
              <span>Execution is blocked. Do not edit or re-run applied migrations blindly. Restore the matching migration source or resolve the unknown/mismatched history before proceeding.</span>
            </div>
          )}

          {hasUnverified && (
            <div className="notice notice-warning">
              <strong>Legacy migrations are not checksum-verified</strong>
              <span>Previously applied migrations predate checksum recording. The new checksum migration will enable verification for future migrations; existing records are marked unverified rather than assumed to match.</span>
            </div>
          )}

          <section className="migration-table-card">
            <div className="migration-table-heading">
              <div>
                <h2>Migration history</h2>
                <p>Applied timestamps are displayed in Kenya time. Migration SQL is never edited from this screen.</p>
              </div>
              <button
                className="button button-primary"
                onClick={() => { setShowConfirm((v) => !v); setConfirmation(''); setError(''); }}
                disabled={!canRun}
                title={hasDrift ? 'Resolve migration history drift first' : !status.execution_enabled ? 'Enable migration execution on the backend first' : undefined}
              >
                {running ? 'Running migrations…' : activeRunStale ? 'Recover stale run and retry' : 'Run pending migrations'}
              </button>
            </div>

            {showConfirm && (
              <div className="migration-confirm">
                <div>
                  <strong>Confirm a schema-changing operation</strong>
                  <p>This applies {pendingVersions.length} pending migration(s) to <code>{status.target_database}</code>. MySQL DDL may partially commit if a migration fails. Back up the database and review pending versions before proceeding.</p>
                  <p>Type <code>{CONFIRMATION}</code> to continue.</p>
                </div>
                <label htmlFor="migration-confirmation">Confirmation phrase</label>
                <input
                  id="migration-confirmation"
                  value={confirmation}
                  onChange={(event) => setConfirmation(event.target.value)}
                  autoComplete="off"
                  spellCheck={false}
                  placeholder={CONFIRMATION}
                />
                <div className="migration-confirm-actions">
                  <button className="button button-secondary" onClick={() => { setShowConfirm(false); setConfirmation(''); }} disabled={running}>Cancel</button>
                  <button className="button button-danger" onClick={() => void runMigrations()} disabled={confirmation !== CONFIRMATION || running}>
                    {running ? 'Applying…' : `Apply ${pendingVersions.length} migration(s)`}
                  </button>
                </div>
              </div>
            )}

            <div className="migration-table-scroll">
              <table className="migration-table">
                <thead>
                  <tr>
                    <th>Version</th>
                    <th>Migration</th>
                    <th>Status</th>
                    <th>Integrity</th>
                    <th>Applied at (EAT)</th>
                  </tr>
                </thead>
                <tbody>
                  {(report?.migrations ?? []).map((migration) => (
                    <tr key={migration.version}>
                      <td><code>{String(migration.version).padStart(4, '0')}</code></td>
                      <td className="migration-name">{migration.name}</td>
                      <td>
                        <span className={`status-pill ${migration.applied ? 'status-applied' : 'status-pending'}`}>
                          <span aria-hidden="true">{migration.applied ? '✓' : '•'}</span>
                          {migration.applied ? 'Applied' : 'Pending'}
                        </span>
                      </td>
                      <td>
                        <span className={`integrity-label ${migration.drift ? 'integrity-error' : migration.checksum_status === 'MATCH' ? 'integrity-ok' : 'integrity-muted'}`}>
                          {migration.checksum_status.replaceAll('_', ' ')}
                        </span>
                      </td>
                      <td>{migration.applied_at ? formatLocalDate(migration.applied_at) : '—'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {(report?.migrations.length ?? 0) === 0 && (
                <div className="migrations-empty"><h2>No migration files found</h2><p>This backend build has no embedded SQL migrations.</p></div>
              )}
            </div>
          </section>
          <p className="migration-footnote">The migration API is restricted to authenticated platform administrators, protected by a server-side feature flag, serialized with the same MySQL advisory lock as the CLI, and audited before and after execution.</p>
        </>
      )}
    </div>
  );
}
