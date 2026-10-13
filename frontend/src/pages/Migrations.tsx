import { useState } from 'react';

interface Migration {
  version: string;
  name: string;
  applied: boolean;
  appliedAt?: string;
}

// Mock data - in production, this would come from the backend
const mockMigrations: Migration[] = [
  { version: '0001', name: 'foundation.sql', applied: true, appliedAt: '2026-10-13 10:00:00' },
  { version: '0002', name: 'permissions.sql', applied: true, appliedAt: '2026-10-13 10:00:01' },
  { version: '0003', name: 'core_workflow.sql', applied: true, appliedAt: '2026-10-13 10:00:02' },
  { version: '0004', name: 'rbac_hardening.sql', applied: true, appliedAt: '2026-10-13 10:00:03' },
  { version: '0005', name: 'refresh_tokens.sql', applied: true, appliedAt: '2026-10-13 10:00:04' },
  { version: '0006', name: 'hash_credential_tokens.sql', applied: true, appliedAt: '2026-10-13 10:00:05' },
  { version: '0007', name: 'revocable_sessions.sql', applied: true, appliedAt: '2026-10-13 10:00:06' },
  { version: '0008', name: 'workflow_integrity.sql', applied: true, appliedAt: '2026-10-13 10:00:07' },
  { version: '0009', name: 'notifications_platform_tenants.sql', applied: true, appliedAt: '2026-10-13 10:00:08' },
];

export default function Migrations() {
  const [migrations] = useState<Migration[]>(mockMigrations);
  const [isRunning, setIsRunning] = useState(false);

  const handleRunMigrations = async () => {
    if (!confirm('Are you sure you want to run pending migrations?')) return;
    setIsRunning(true);
    // In production, this would call the backend migration endpoint
    setTimeout(() => {
      setIsRunning(false);
      alert('Migrations completed successfully');
    }, 2000);
  };

  const appliedCount = migrations.filter(m => m.applied).length;
  const pendingCount = migrations.length - appliedCount;

  return (
    <div style={{ padding: '2rem', maxWidth: '1200px', margin: '0 auto' }}>
      <div style={{ marginBottom: '2rem' }}>
        <h1 style={{ fontSize: '1.875rem', fontWeight: 'bold', margin: '0.5rem 0' }}>Database Migrations</h1>
        <p style={{ color: '#6b7280', margin: 0 }}>Manage database schema migrations</p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: '1rem', marginBottom: '2rem' }}>
        <div style={{ backgroundColor: 'white', padding: '1.5rem', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
          <p style={{ fontSize: '0.75rem', color: '#6b7280', margin: '0 0 0.5rem 0' }}>Total Migrations</p>
          <p style={{ fontSize: '2rem', fontWeight: 'bold', margin: 0 }}>{migrations.length}</p>
        </div>
        <div style={{ backgroundColor: 'white', padding: '1.5rem', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
          <p style={{ fontSize: '0.75rem', color: '#6b7280', margin: '0 0 0.5rem 0' }}>Applied</p>
          <p style={{ fontSize: '2rem', fontWeight: 'bold', margin: 0, color: '#10b981' }}>{appliedCount}</p>
        </div>
        <div style={{ backgroundColor: 'white', padding: '1.5rem', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
          <p style={{ fontSize: '0.75rem', color: '#6b7280', margin: '0 0 0.5rem 0' }}>Pending</p>
          <p style={{ fontSize: '2rem', fontWeight: 'bold', margin: 0, color: pendingCount > 0 ? '#f59e0b' : '#6b7280' }}>{pendingCount}</p>
        </div>
      </div>

      <div style={{ backgroundColor: 'white', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)', overflow: 'hidden' }}>
        <div style={{ padding: '1rem 1.5rem', borderBottom: '1px solid #e5e7eb', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 style={{ fontSize: '1rem', fontWeight: '600', margin: 0 }}>Migration History</h2>
          {pendingCount > 0 && (
            <button
              onClick={handleRunMigrations}
              disabled={isRunning}
              style={{
                padding: '0.5rem 1rem',
                backgroundColor: isRunning ? '#ccc' : '#2563eb',
                color: 'white',
                border: 'none',
                borderRadius: '4px',
                fontSize: '0.875rem',
                fontWeight: '500',
                cursor: isRunning ? 'not-allowed' : 'pointer',
              }}
            >
              {isRunning ? 'Running...' : 'Run Pending Migrations'}
            </button>
          )}
        </div>

        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr style={{ backgroundColor: '#f9fafb', borderBottom: '1px solid #e5e7eb' }}>
              <th style={{ padding: '0.75rem 1rem', textAlign: 'left', fontSize: '0.75rem', fontWeight: '600', textTransform: 'uppercase', color: '#6b7280' }}>Version</th>
              <th style={{ padding: '0.75rem 1rem', textAlign: 'left', fontSize: '0.75rem', fontWeight: '600', textTransform: 'uppercase', color: '#6b7280' }}>Name</th>
              <th style={{ padding: '0.75rem 1rem', textAlign: 'left', fontSize: '0.75rem', fontWeight: '600', textTransform: 'uppercase', color: '#6b7280' }}>Status</th>
              <th style={{ padding: '0.75rem 1rem', textAlign: 'left', fontSize: '0.75rem', fontWeight: '600', textTransform: 'uppercase', color: '#6b7280' }}>Applied At</th>
            </tr>
          </thead>
          <tbody>
            {migrations.map((migration) => (
              <tr key={migration.version} style={{ borderBottom: '1px solid #e5e7eb' }}>
                <td style={{ padding: '0.75rem 1rem', fontSize: '0.875rem', color: '#374151', fontFamily: 'monospace' }}>{migration.version}</td>
                <td style={{ padding: '0.75rem 1rem', fontSize: '0.875rem', color: '#374151' }}>{migration.name}</td>
                <td style={{ padding: '0.75rem 1rem' }}>
                  <span style={{
                    display: 'inline-block',
                    padding: '0.25rem 0.5rem',
                    borderRadius: '9999px',
                    fontSize: '0.75rem',
                    fontWeight: '500',
                    backgroundColor: migration.applied ? '#d1fae5' : '#fef3c7',
                    color: migration.applied ? '#059669' : '#d97706',
                  }}>
                    {migration.applied ? 'Applied' : 'Pending'}
                  </span>
                </td>
                <td style={{ padding: '0.75rem 1rem', fontSize: '0.875rem', color: '#6b7280' }}>
                  {migration.appliedAt || '-'}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div style={{ marginTop: '2rem', padding: '1rem', backgroundColor: '#fef3c7', borderRadius: '8px', border: '1px solid #fcd34d' }}>
        <p style={{ fontSize: '0.875rem', color: '#92400e', margin: 0 }}>
          <strong>Note:</strong> This is a frontend-only view. In production, migrations should be run via the backend CLI or a dedicated migration endpoint with proper authentication and validation.
        </p>
      </div>
    </div>
  );
}
