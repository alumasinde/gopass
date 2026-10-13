import { useState } from 'react';
import type { Organization } from '../lib/platform-api';
import { platformOrganizations } from '../lib/platform-api';
import { formatLocalDate } from '../lib/dateUtils';

interface TenantListProps {
  tenants: Organization[];
  onAction: () => void;
  onRowClick?: (id: number) => void;
}

export default function TenantList({ tenants, onAction, onRowClick }: TenantListProps) {
  const [actionLoading, setActionLoading] = useState<{ [key: number]: string }>({});

  const handleSuspend = async (id: number) => {
    if (!confirm('Are you sure you want to suspend this tenant?')) return;
    
    setActionLoading(prev => ({ ...prev, [id]: 'suspend' }));
    try {
      await platformOrganizations.suspend(id);
      onAction();
    } catch (error) {
      alert('Failed to suspend tenant');
    } finally {
      setActionLoading(prev => ({ ...prev, [id]: '' }));
    }
  };

  const handleResume = async (id: number) => {
    if (!confirm('Are you sure you want to resume this tenant?')) return;
    
    setActionLoading(prev => ({ ...prev, [id]: 'resume' }));
    try {
      await platformOrganizations.resume(id);
      onAction();
    } catch (error) {
      alert('Failed to resume tenant');
    } finally {
      setActionLoading(prev => ({ ...prev, [id]: '' }));
    }
  };

  const handleDeprovision = async (id: number) => {
    if (!confirm('Are you sure you want to deprovision this tenant? This action cannot be undone.')) return;
    
    setActionLoading(prev => ({ ...prev, [id]: 'deprovision' }));
    try {
      await platformOrganizations.deprovision(id);
      onAction();
    } catch (error) {
      alert('Failed to deprovision tenant');
    } finally {
      setActionLoading(prev => ({ ...prev, [id]: '' }));
    }
  };

  const handleRetryProvisioning = async (id: number) => {
    setActionLoading(prev => ({ ...prev, [id]: 'retry' }));
    try {
      await platformOrganizations.retryProvisioning(id);
      onAction();
    } catch (error) {
      alert('Failed to retry provisioning');
    } finally {
      setActionLoading(prev => ({ ...prev, [id]: '' }));
    }
  };

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

  if (tenants.length === 0) {
    return (
      <div style={{ backgroundColor: 'white', padding: '2rem', borderRadius: '8px', textAlign: 'center', color: '#6b7280' }}>
        <p>No tenants found. Create your first tenant to get started.</p>
      </div>
    );
  }

  return (
    <div style={{ backgroundColor: 'white', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)', overflow: 'hidden' }}>
      <table style={{ width: '100%', borderCollapse: 'collapse' }}>
        <thead>
          <tr style={{ backgroundColor: '#f9fafb', borderBottom: '1px solid #e5e7eb' }}>
            <th style={{ padding: '0.75rem 1rem', textAlign: 'left', fontSize: '0.75rem', fontWeight: '600', textTransform: 'uppercase', color: '#6b7280' }}>ID</th>
            <th style={{ padding: '0.75rem 1rem', textAlign: 'left', fontSize: '0.75rem', fontWeight: '600', textTransform: 'uppercase', color: '#6b7280' }}>Name</th>
            <th style={{ padding: '0.75rem 1rem', textAlign: 'left', fontSize: '0.75rem', fontWeight: '600', textTransform: 'uppercase', color: '#6b7280' }}>Slug</th>
            <th style={{ padding: '0.75rem 1rem', textAlign: 'left', fontSize: '0.75rem', fontWeight: '600', textTransform: 'uppercase', color: '#6b7280' }}>Lifecycle</th>
            <th style={{ padding: '0.75rem 1rem', textAlign: 'left', fontSize: '0.75rem', fontWeight: '600', textTransform: 'uppercase', color: '#6b7280' }}>Provisioning</th>
            <th style={{ padding: '0.75rem 1rem', textAlign: 'left', fontSize: '0.75rem', fontWeight: '600', textTransform: 'uppercase', color: '#6b7280' }}>Database</th>
            <th style={{ padding: '0.75rem 1rem', textAlign: 'left', fontSize: '0.75rem', fontWeight: '600', textTransform: 'uppercase', color: '#6b7280' }}>Created</th>
            <th style={{ padding: '0.75rem 1rem', textAlign: 'right', fontSize: '0.75rem', fontWeight: '600', textTransform: 'uppercase', color: '#6b7280' }}>Actions</th>
          </tr>
        </thead>
        <tbody>
          {tenants.map((tenant) => (
            <tr 
              key={tenant.id} 
              style={{ borderBottom: '1px solid #e5e7eb', cursor: onRowClick ? 'pointer' : 'default' }}
              onClick={() => onRowClick?.(tenant.id)}
            >
              <td style={{ padding: '0.75rem 1rem', fontSize: '0.875rem', color: '#374151' }}>{tenant.id}</td>
              <td style={{ padding: '0.75rem 1rem', fontSize: '0.875rem', color: '#374151', fontWeight: '500' }}>{tenant.name}</td>
              <td style={{ padding: '0.75rem 1rem', fontSize: '0.875rem', color: '#6b7280' }}>{tenant.slug}</td>
              <td style={{ padding: '0.75rem 1rem' }}>
                <span style={{
                  display: 'inline-block',
                  padding: '0.25rem 0.5rem',
                  borderRadius: '9999px',
                  fontSize: '0.75rem',
                  fontWeight: '500',
                  backgroundColor: `${getStatusColor(tenant.lifecycle_status)}20`,
                  color: getStatusColor(tenant.lifecycle_status),
                }}>
                  {tenant.lifecycle_status}
                </span>
              </td>
              <td style={{ padding: '0.75rem 1rem' }}>
                <span style={{
                  display: 'inline-block',
                  padding: '0.25rem 0.5rem',
                  borderRadius: '9999px',
                  fontSize: '0.75rem',
                  fontWeight: '500',
                  backgroundColor: `${getProvisioningColor(tenant.provisioning_status)}20`,
                  color: getProvisioningColor(tenant.provisioning_status),
                }}>
                  {tenant.provisioning_status}
                </span>
                {tenant.provisioning_error && (
                  <div style={{ fontSize: '0.75rem', color: '#ef4444', marginTop: '0.25rem' }}>{tenant.provisioning_error}</div>
                )}
              </td>
              <td style={{ padding: '0.75rem 1rem', fontSize: '0.875rem', color: '#6b7280' }}>{tenant.database_mode}</td>
              <td style={{ padding: '0.75rem 1rem', fontSize: '0.875rem', color: '#6b7280' }}>
                {formatLocalDate(tenant.created_at)}
              </td>
              <td style={{ padding: '0.75rem 1rem', textAlign: 'right' }}>
                <div style={{ display: 'flex', gap: '0.5rem', justifyContent: 'flex-end', flexWrap: 'wrap' }}>
                  {onRowClick && (
                    <button
                      type="button"
                      onClick={(event) => { event.stopPropagation(); onRowClick(tenant.id); }}
                      style={{ padding: '0.25rem 0.5rem', backgroundColor: '#eff6ff', color: '#1d4ed8', border: '1px solid #bfdbfe', borderRadius: '4px', fontSize: '0.75rem', cursor: 'pointer' }}
                    >
                      View details
                    </button>
                  )}
                  {tenant.lifecycle_status === 'ACTIVE' && (
                    <button
                      onClick={(event) => { event.stopPropagation(); handleSuspend(tenant.id); }}
                      disabled={!!actionLoading[tenant.id]}
                      style={{
                        padding: '0.25rem 0.5rem',
                        backgroundColor: '#fef3c7',
                        color: '#d97706',
                        border: 'none',
                        borderRadius: '4px',
                        fontSize: '0.75rem',
                        cursor: actionLoading[tenant.id] ? 'not-allowed' : 'pointer',
                      }}
                    >
                      {actionLoading[tenant.id] === 'suspend' ? '...' : 'Suspend'}
                    </button>
                  )}
                  {tenant.lifecycle_status === 'SUSPENDED' && (
                    <button
                      onClick={(event) => { event.stopPropagation(); handleResume(tenant.id); }}
                      disabled={!!actionLoading[tenant.id]}
                      style={{
                        padding: '0.25rem 0.5rem',
                        backgroundColor: '#d1fae5',
                        color: '#059669',
                        border: 'none',
                        borderRadius: '4px',
                        fontSize: '0.75rem',
                        cursor: actionLoading[tenant.id] ? 'not-allowed' : 'pointer',
                      }}
                    >
                      {actionLoading[tenant.id] === 'resume' ? '...' : 'Resume'}
                    </button>
                  )}
                  {(tenant.lifecycle_status === 'ACTIVE' || tenant.lifecycle_status === 'SUSPENDED') && (
                    <button
                      onClick={(event) => { event.stopPropagation(); handleDeprovision(tenant.id); }}
                      disabled={!!actionLoading[tenant.id]}
                      style={{
                        padding: '0.25rem 0.5rem',
                        backgroundColor: '#fee2e2',
                        color: '#dc2626',
                        border: 'none',
                        borderRadius: '4px',
                        fontSize: '0.75rem',
                        cursor: actionLoading[tenant.id] ? 'not-allowed' : 'pointer',
                      }}
                    >
                      {actionLoading[tenant.id] === 'deprovision' ? '...' : 'Deprovision'}
                    </button>
                  )}
                  {(tenant.provisioning_status === 'FAILED' || tenant.provisioning_status === 'BLOCKED') && (
                    <button
                      onClick={(event) => { event.stopPropagation(); handleRetryProvisioning(tenant.id); }}
                      disabled={!!actionLoading[tenant.id]}
                      style={{
                        padding: '0.25rem 0.5rem',
                        backgroundColor: '#dbeafe',
                        color: '#2563eb',
                        border: 'none',
                        borderRadius: '4px',
                        fontSize: '0.75rem',
                        cursor: actionLoading[tenant.id] ? 'not-allowed' : 'pointer',
                      }}
                    >
                      {actionLoading[tenant.id] === 'retry' ? '...' : 'Retry'}
                    </button>
                  )}
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
