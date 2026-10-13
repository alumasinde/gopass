import { useState, useEffect } from 'react';
import type { FormEvent } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { platformOrganizations } from '../lib/platform-api';
import type { Organization } from '../lib/platform-api';
import { formatLocalDate } from '../lib/dateUtils';
import { apiErrorMessage } from '../lib/api';

export default function TenantDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [tenant, setTenant] = useState<Organization | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState('');
  const [databaseDSN, setDatabaseDSN] = useState('');
  const [savingDatabase, setSavingDatabase] = useState(false);
  const [databaseMessage, setDatabaseMessage] = useState('');
  const [databaseError, setDatabaseError] = useState('');

  useEffect(() => {
    const fetchTenant = async () => {
      if (!id) return;
      try {
        setIsLoading(true);
        setError('');
        const data = await platformOrganizations.get(Number(id));
        setTenant(data);
      } catch (err: any) {
        setError(apiErrorMessage(err, 'Failed to fetch tenant details'));
      } finally {
        setIsLoading(false);
      }
    };

    fetchTenant();
  }, [id]);

  const handleSuspend = async () => {
    if (!tenant || !confirm('Are you sure you want to suspend this tenant?')) return;
    try {
      await platformOrganizations.suspend(tenant.id);
      const updated = await platformOrganizations.get(tenant.id);
      setTenant(updated);
    } catch (err: any) {
      alert(apiErrorMessage(err, 'Failed to suspend tenant'));
    }
  };

  const handleResume = async () => {
    if (!tenant || !confirm('Are you sure you want to resume this tenant?')) return;
    try {
      await platformOrganizations.resume(tenant.id);
      const updated = await platformOrganizations.get(tenant.id);
      setTenant(updated);
    } catch (err: any) {
      alert(apiErrorMessage(err, 'Failed to resume tenant'));
    }
  };

  const handleDeprovision = async () => {
    if (!tenant || !confirm('Are you sure you want to deprovision this tenant? This action cannot be undone.')) return;
    try {
      await platformOrganizations.deprovision(tenant.id);
      const updated = await platformOrganizations.get(tenant.id);
      setTenant(updated);
    } catch (err: any) {
      alert(apiErrorMessage(err, 'Failed to deprovision tenant'));
    }
  };

  const handleRetryProvisioning = async () => {
    if (!tenant) return;
    try {
      const result = await platformOrganizations.retryProvisioning(tenant.id);
      if (result.id !== tenant.id) {
        navigate(`/platform/tenants/${result.id}`, { replace: true });
        const updated = await platformOrganizations.get(result.id);
        setTenant(updated);
      } else {
        const updated = await platformOrganizations.get(tenant.id);
        setTenant(updated);
      }
    } catch (err: any) {
      alert(apiErrorMessage(err, 'Failed to retry provisioning'));
    }
  };

  const handleSaveDatabase = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!tenant || !databaseDSN.trim()) return;
    setSavingDatabase(true);
    setDatabaseError('');
    setDatabaseMessage('');
    try {
      await platformOrganizations.setDatabaseConfig(tenant.id, databaseDSN.trim());
      setDatabaseDSN('');
      setDatabaseMessage('Database credentials validated and stored encrypted. Provisioning will run in the background.');
      const updated = await platformOrganizations.get(tenant.id);
      setTenant(updated);
    } catch (err: any) {
      setDatabaseError(apiErrorMessage(err, 'Failed to configure the dedicated database'));
    } finally {
      setSavingDatabase(false);
    }
  };

  if (isLoading) {
    return (
      <div style={{ padding: '2rem', textAlign: 'center' }}>
        Loading...
      </div>
    );
  }

  if (error || !tenant) {
    return (
      <div style={{ padding: '2rem' }}>
        <div style={{ backgroundColor: '#fee', color: '#c33', padding: '0.75rem', borderRadius: '4px', marginBottom: '1rem' }}>
          {error || 'Tenant not found'}
        </div>
        <button
          onClick={() => navigate('/platform/tenants')}
          style={{
            padding: '0.5rem 1rem',
            backgroundColor: '#2563eb',
            color: 'white',
            border: 'none',
            borderRadius: '4px',
            cursor: 'pointer',
          }}
        >
          Back to Tenants
        </button>
      </div>
    );
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'ACTIVE': return '#10b981';
      case 'SUSPENDED': return '#f59e0b';
      case 'DEPROVISIONED': return '#6b7280';
      case 'PROVISIONING': return '#3b82f6';
      default: return '#6b7280';
    }
  };

  const getProvisioningColor = (status: string) => {
    switch (status) {
      case 'READY': return '#10b981';
      case 'PENDING': return '#3b82f6';
      case 'BOOTSTRAPPING': return '#8b5cf6';
      case 'FAILED': return '#ef4444';
      case 'BLOCKED': return '#f59e0b';
      default: return '#6b7280';
    }
  };

  return (
    <div style={{ padding: '2rem', maxWidth: '1200px', margin: '0 auto' }}>
      <div style={{ marginBottom: '2rem' }}>
        <button
          onClick={() => navigate('/platform/tenants')}
          style={{
            padding: '0.5rem 1rem',
            backgroundColor: 'white',
            color: '#374151',
            border: '1px solid #d1d5db',
            borderRadius: '4px',
            cursor: 'pointer',
            marginBottom: '1rem',
          }}
        >
          ← Back to Tenants
        </button>
        <h1 style={{ fontSize: '1.875rem', fontWeight: 'bold', margin: '0.5rem 0' }}>{tenant.name}</h1>
        <p style={{ color: '#6b7280', margin: 0 }}>Slug: {tenant.slug}</p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))', gap: '1.5rem', marginBottom: '2rem' }}>
        <div style={{ backgroundColor: 'white', padding: '1.5rem', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
          <h3 style={{ fontSize: '1rem', fontWeight: '600', marginBottom: '1rem' }}>Lifecycle Status</h3>
          <div style={{ display: 'inline-block', padding: '0.5rem 1rem', borderRadius: '9999px', backgroundColor: `${getStatusColor(tenant.lifecycle_status)}20`, color: getStatusColor(tenant.lifecycle_status), fontWeight: '500' }}>
            {tenant.lifecycle_status}
          </div>
        </div>

        <div style={{ backgroundColor: 'white', padding: '1.5rem', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
          <h3 style={{ fontSize: '1rem', fontWeight: '600', marginBottom: '1rem' }}>Provisioning Status</h3>
          <div style={{ display: 'inline-block', padding: '0.5rem 1rem', borderRadius: '9999px', backgroundColor: `${getProvisioningColor(tenant.provisioning_status)}20`, color: getProvisioningColor(tenant.provisioning_status), fontWeight: '500' }}>
            {tenant.provisioning_status}
          </div>
          {tenant.provisioning_error && (
            <p style={{ color: '#ef4444', fontSize: '0.875rem', marginTop: '0.5rem' }}>{tenant.provisioning_error}</p>
          )}
        </div>

        <div style={{ backgroundColor: 'white', padding: '1.5rem', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
          <h3 style={{ fontSize: '1rem', fontWeight: '600', marginBottom: '1rem' }}>Database Mode</h3>
          <p style={{ fontSize: '1.125rem', fontWeight: '500', margin: 0 }}>{tenant.database_mode}</p>
        </div>
      </div>

      {tenant.database_mode === 'DEDICATED' && (!tenant.database_configured || tenant.provisioning_status === 'FAILED' || tenant.provisioning_status === 'BLOCKED') && tenant.lifecycle_status !== 'DEPROVISIONED' && (
        <div style={{ backgroundColor: 'white', padding: '1.5rem', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)', marginBottom: '2rem' }}>
          <h3 style={{ fontSize: '1rem', fontWeight: '600', marginBottom: '0.5rem' }}>Dedicated database configuration</h3>
          <p style={{ color: '#6b7280', fontSize: '0.875rem', marginTop: 0 }}>Enter a MySQL DSN for an empty database or an existing GoPass tenant schema. Credentials are encrypted before they are stored. Do not use the platform or shared tenant database.</p>
          {databaseError && <div role="alert" style={{ backgroundColor: '#fee2e2', color: '#991b1b', padding: '0.75rem', borderRadius: '4px', marginBottom: '1rem' }}>{databaseError}</div>}
          {databaseMessage && <div role="status" style={{ backgroundColor: '#dcfce7', color: '#166534', padding: '0.75rem', borderRadius: '4px', marginBottom: '1rem' }}>{databaseMessage}</div>}
          <form onSubmit={handleSaveDatabase}>
            <label htmlFor="tenant-database-dsn" style={{ display: 'block', fontSize: '0.875rem', fontWeight: 500, marginBottom: '0.5rem' }}>MySQL DSN</label>
            <input id="tenant-database-dsn" type="password" autoComplete="new-password" value={databaseDSN} onChange={event => setDatabaseDSN(event.target.value)} required placeholder="user:password@tcp(host:3306)/tenant_db?parseTime=true" style={{ width: '100%', boxSizing: 'border-box', padding: '0.75rem', border: '1px solid #d1d5db', borderRadius: '6px', marginBottom: '1rem' }} />
            <button type="submit" disabled={savingDatabase} style={{ padding: '0.65rem 1rem', backgroundColor: savingDatabase ? '#9ca3af' : '#2563eb', color: 'white', border: 0, borderRadius: '6px', cursor: savingDatabase ? 'not-allowed' : 'pointer' }}>{savingDatabase ? 'Validating…' : 'Save and provision database'}</button>
          </form>
        </div>
      )}

      <div style={{ backgroundColor: 'white', padding: '1.5rem', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)', marginBottom: '2rem' }}>
        <h3 style={{ fontSize: '1rem', fontWeight: '600', marginBottom: '1rem' }}>Timeline</h3>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: '1rem' }}>
          <div>
            <p style={{ fontSize: '0.75rem', color: '#6b7280', margin: '0 0 0.25rem 0' }}>Created</p>
            <p style={{ fontSize: '0.875rem', margin: 0 }}>{formatLocalDate(tenant.created_at)}</p>
          </div>
          <div>
            <p style={{ fontSize: '0.75rem', color: '#6b7280', margin: '0 0 0.25rem 0' }}>Updated</p>
            <p style={{ fontSize: '0.875rem', margin: 0 }}>{formatLocalDate(tenant.updated_at)}</p>
          </div>
          {tenant.provisioned_at && (
            <div>
              <p style={{ fontSize: '0.75rem', color: '#6b7280', margin: '0 0 0.25rem 0' }}>Provisioned</p>
              <p style={{ fontSize: '0.875rem', margin: 0 }}>{formatLocalDate(tenant.provisioned_at)}</p>
            </div>
          )}
          {tenant.suspended_at && (
            <div>
              <p style={{ fontSize: '0.75rem', color: '#6b7280', margin: '0 0 0.25rem 0' }}>Suspended</p>
              <p style={{ fontSize: '0.875rem', margin: 0 }}>{formatLocalDate(tenant.suspended_at)}</p>
            </div>
          )}
        </div>
      </div>

      <div style={{ backgroundColor: 'white', padding: '1.5rem', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
        <h3 style={{ fontSize: '1rem', fontWeight: '600', marginBottom: '1rem' }}>Actions</h3>
        <div style={{ display: 'flex', gap: '1rem', flexWrap: 'wrap' }}>
          {tenant.lifecycle_status === 'ACTIVE' && (
            <button
              onClick={handleSuspend}
              style={{
                padding: '0.5rem 1rem',
                backgroundColor: '#fef3c7',
                color: '#d97706',
                border: 'none',
                borderRadius: '4px',
                fontSize: '0.875rem',
                fontWeight: '500',
                cursor: 'pointer',
              }}
            >
              Suspend
            </button>
          )}
          {tenant.lifecycle_status === 'SUSPENDED' && (
            <button
              onClick={handleResume}
              style={{
                padding: '0.5rem 1rem',
                backgroundColor: '#d1fae5',
                color: '#059669',
                border: 'none',
                borderRadius: '4px',
                fontSize: '0.875rem',
                fontWeight: '500',
                cursor: 'pointer',
              }}
            >
              Resume
            </button>
          )}
          {(tenant.lifecycle_status === 'ACTIVE' || tenant.lifecycle_status === 'SUSPENDED') && (
            <button
              onClick={handleDeprovision}
              style={{
                padding: '0.5rem 1rem',
                backgroundColor: '#fee2e2',
                color: '#dc2626',
                border: 'none',
                borderRadius: '4px',
                fontSize: '0.875rem',
                fontWeight: '500',
                cursor: 'pointer',
              }}
            >
              Deprovision
            </button>
          )}
          {(tenant.provisioning_status === 'FAILED' || tenant.provisioning_status === 'BLOCKED') && (
            <button
              onClick={handleRetryProvisioning}
              style={{
                padding: '0.5rem 1rem',
                backgroundColor: '#dbeafe',
                color: '#2563eb',
                border: 'none',
                borderRadius: '4px',
                fontSize: '0.875rem',
                fontWeight: '500',
                cursor: 'pointer',
              }}
            >
              Retry Provisioning
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
