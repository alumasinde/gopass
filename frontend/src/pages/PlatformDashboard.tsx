import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { platformOrganizations } from '../lib/platform-api';
import type { Organization } from '../lib/platform-api';
import { apiErrorMessage } from '../lib/api';

export default function PlatformDashboard() {
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState('');
  const navigate = useNavigate();

  const fetchOrganizations = async () => {
    try {
      setIsLoading(true);
      setError('');
      const data = await platformOrganizations.list();
      setOrganizations(data);
    } catch (err: any) {
      setError(apiErrorMessage(err, 'Failed to fetch organizations'));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchOrganizations();
  }, []);

  return (
    <div style={{ padding: '2rem', maxWidth: '1200px', margin: '0 auto' }}>
      <div style={{ marginBottom: '2rem' }}>
        <h1 style={{ fontSize: '1.875rem', fontWeight: 'bold', margin: '0.5rem 0' }}>Dashboard</h1>
        <p style={{ color: '#6b7280', margin: 0 }}>Platform Admin Overview</p>
      </div>

      {error && (
        <div style={{ backgroundColor: '#fee', color: '#c33', padding: '0.75rem', borderRadius: '4px', marginBottom: '1rem' }}>
          {error}
        </div>
      )}

      {isLoading ? (
        <div style={{ textAlign: 'center', padding: '2rem', color: '#6b7280' }}>Loading...</div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(250px, 1fr))', gap: '1.5rem' }}>
          <div style={{ backgroundColor: 'white', padding: '1.5rem', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
            <p style={{ fontSize: '0.75rem', color: '#6b7280', margin: '0 0 0.5rem 0' }}>Total Tenants</p>
            <p style={{ fontSize: '2rem', fontWeight: 'bold', margin: 0 }}>{organizations.length}</p>
          </div>
          <div style={{ backgroundColor: 'white', padding: '1.5rem', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
            <p style={{ fontSize: '0.75rem', color: '#6b7280', margin: '0 0 0.5rem 0' }}>Active Tenants</p>
            <p style={{ fontSize: '2rem', fontWeight: 'bold', margin: 0, color: '#10b981' }}>
              {organizations.filter(o => o.is_active && o.lifecycle_status === 'ACTIVE').length}
            </p>
          </div>
          <div style={{ backgroundColor: 'white', padding: '1.5rem', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
            <p style={{ fontSize: '0.75rem', color: '#6b7280', margin: '0 0 0.5rem 0' }}>Provisioning</p>
            <p style={{ fontSize: '2rem', fontWeight: 'bold', margin: 0, color: '#3b82f6' }}>
              {organizations.filter(o => o.provisioning_status === 'PENDING').length}
            </p>
          </div>
          <div style={{ backgroundColor: 'white', padding: '1.5rem', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
            <p style={{ fontSize: '0.75rem', color: '#6b7280', margin: '0 0 0.5rem 0' }}>Failed</p>
            <p style={{ fontSize: '2rem', fontWeight: 'bold', margin: 0, color: '#ef4444' }}>
              {organizations.filter(o => o.provisioning_status === 'FAILED').length}
            </p>
          </div>
        </div>
      )}

      <div style={{ marginTop: '2rem', backgroundColor: 'white', padding: '1.5rem', borderRadius: '8px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
          <h2 style={{ fontSize: '1rem', fontWeight: '600', margin: 0 }}>Recent Tenants</h2>
          <button
            onClick={() => navigate('/platform/tenants')}
            style={{
              padding: '0.5rem 1rem',
              backgroundColor: '#2563eb',
              color: 'white',
              border: 'none',
              borderRadius: '4px',
              fontSize: '0.875rem',
              fontWeight: '500',
              cursor: 'pointer',
            }}
          >
            View All
          </button>
        </div>
        {!isLoading && organizations.length > 0 ? (
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ backgroundColor: '#f9fafb', borderBottom: '1px solid #e5e7eb' }}>
                  <th style={{ padding: '0.75rem 1rem', textAlign: 'left', fontSize: '0.75rem', fontWeight: '600', textTransform: 'uppercase', color: '#6b7280' }}>Name</th>
                  <th style={{ padding: '0.75rem 1rem', textAlign: 'left', fontSize: '0.75rem', fontWeight: '600', textTransform: 'uppercase', color: '#6b7280' }}>Status</th>
                  <th style={{ padding: '0.75rem 1rem', textAlign: 'left', fontSize: '0.75rem', fontWeight: '600', textTransform: 'uppercase', color: '#6b7280' }}>Created</th>
                </tr>
              </thead>
              <tbody>
                {organizations.slice(0, 5).map((tenant) => (
                  <tr key={tenant.id} style={{ borderBottom: '1px solid #e5e7eb', cursor: 'pointer' }} onClick={() => navigate(`/platform/tenants/${tenant.id}`)}>
                    <td style={{ padding: '0.75rem 1rem', fontSize: '0.875rem', color: '#374151', fontWeight: '500' }}>{tenant.name}</td>
                    <td style={{ padding: '0.75rem 1rem' }}>
                      <span style={{
                        display: 'inline-block',
                        padding: '0.25rem 0.5rem',
                        borderRadius: '9999px',
                        fontSize: '0.75rem',
                        fontWeight: '500',
                        backgroundColor: tenant.lifecycle_status === 'ACTIVE' ? '#d1fae5' : '#fef3c7',
                        color: tenant.lifecycle_status === 'ACTIVE' ? '#059669' : '#d97706',
                      }}>
                        {tenant.lifecycle_status}
                      </span>
                    </td>
                    <td style={{ padding: '0.75rem 1rem', fontSize: '0.875rem', color: '#6b7280' }}>
                      {new Date(tenant.created_at).toLocaleDateString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p style={{ color: '#6b7280', textAlign: 'center', padding: '2rem' }}>No tenants yet</p>
        )}
      </div>
    </div>
  );
}
