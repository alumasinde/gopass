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
      <div className="extracted-style-113f025f29">
        <p>No tenants found. Create your first tenant to get started.</p>
      </div>
    );
  }

  return (
    <div className="extracted-style-40736e307b">
      <table className="extracted-style-a55f31db10">
        <thead>
          <tr className="extracted-style-e5046e59fb">
            <th className="extracted-style-ea59bf4a41">ID</th>
            <th className="extracted-style-ea59bf4a41">Name</th>
            <th className="extracted-style-ea59bf4a41">Slug</th>
            <th className="extracted-style-ea59bf4a41">Lifecycle</th>
            <th className="extracted-style-ea59bf4a41">Provisioning</th>
            <th className="extracted-style-ea59bf4a41">Database</th>
            <th className="extracted-style-ea59bf4a41">Created</th>
            <th className="extracted-style-105135d318">Actions</th>
          </tr>
        </thead>
        <tbody>
          {tenants.map((tenant) => (
            <tr 
              key={tenant.id} 
              className={onRowClick ? "legacy-tenant-row legacy-tenant-row-clickable" : "legacy-tenant-row"}
              onClick={() => onRowClick?.(tenant.id)}
            >
              <td className="extracted-style-bed1b6355f">{tenant.id}</td>
              <td className="extracted-style-29797852fe">{tenant.name}</td>
              <td className="extracted-style-c613d744ac">{tenant.slug}</td>
              <td className="extracted-style-6aa1ccdbf9">
                <span className={`legacy-status-pill legacy-status-${tenant.lifecycle_status.toLowerCase()}`}>
                  {tenant.lifecycle_status}
                </span>
              </td>
              <td className="extracted-style-6aa1ccdbf9">
                <span className={`legacy-status-pill legacy-provisioning-${tenant.provisioning_status.toLowerCase()}`}>
                  {tenant.provisioning_status}
                </span>
                {tenant.provisioning_error && (
                  <div className="extracted-style-ece7c40dc9">{tenant.provisioning_error}</div>
                )}
              </td>
              <td className="extracted-style-c613d744ac">{tenant.database_mode}</td>
              <td className="extracted-style-c613d744ac">
                {formatLocalDate(tenant.created_at)}
              </td>
              <td className="extracted-style-b15f608197">
                <div className="extracted-style-97eaa8db81">
                  {onRowClick && (
                    <button
                      type="button"
                      onClick={(event) => { event.stopPropagation(); onRowClick(tenant.id); }}
                      className="extracted-style-585a5a0a6f"
                    >
                      View details
                    </button>
                  )}
                  {tenant.lifecycle_status === 'ACTIVE' && (
                    <button
                      onClick={(event) => { event.stopPropagation(); handleSuspend(tenant.id); }}
                      disabled={!!actionLoading[tenant.id]}
                      className="legacy-tenant-action legacy-tenant-action-warning"
                    >
                      {actionLoading[tenant.id] === 'suspend' ? '...' : 'Suspend'}
                    </button>
                  )}
                  {tenant.lifecycle_status === 'SUSPENDED' && (
                    <button
                      onClick={(event) => { event.stopPropagation(); handleResume(tenant.id); }}
                      disabled={!!actionLoading[tenant.id]}
                      className="legacy-tenant-action legacy-tenant-action-success"
                    >
                      {actionLoading[tenant.id] === 'resume' ? '...' : 'Resume'}
                    </button>
                  )}
                  {(tenant.lifecycle_status === 'ACTIVE' || tenant.lifecycle_status === 'SUSPENDED') && (
                    <button
                      onClick={(event) => { event.stopPropagation(); handleDeprovision(tenant.id); }}
                      disabled={!!actionLoading[tenant.id]}
                      className="legacy-tenant-action legacy-tenant-action-danger"
                    >
                      {actionLoading[tenant.id] === 'deprovision' ? '...' : 'Deprovision'}
                    </button>
                  )}
                  {(tenant.provisioning_status === 'FAILED' || tenant.provisioning_status === 'BLOCKED') && (
                    <button
                      onClick={(event) => { event.stopPropagation(); handleRetryProvisioning(tenant.id); }}
                      disabled={!!actionLoading[tenant.id]}
                      className="legacy-tenant-action legacy-tenant-action-info"
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
