import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { platformOrganizations } from '../lib/platform-api';
import type { Organization } from '../lib/platform-api';
import TenantList from '../components/TenantList';
import CreateTenantModal from '../components/CreateTenantModal';
import { apiErrorMessage } from '../lib/api';

export default function Tenants() {
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState('');
  const [showCreateModal, setShowCreateModal] = useState(false);
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

  const handleTenantCreated = () => {
    setShowCreateModal(false);
    fetchOrganizations();
  };

  const handleTenantAction = () => {
    fetchOrganizations();
  };

  return (
    <div style={{ padding: '2rem', maxWidth: '1200px', margin: '0 auto' }}>
      <div style={{ marginBottom: '2rem', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <h1 style={{ fontSize: '1.875rem', fontWeight: 'bold', margin: '0.5rem 0' }}>Tenants</h1>
          <p style={{ color: '#6b7280', margin: 0 }}>Manage tenant organizations</p>
        </div>
        <button
          onClick={() => setShowCreateModal(true)}
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
          Create Tenant
        </button>
      </div>

      {error && (
        <div style={{ backgroundColor: '#fee', color: '#c33', padding: '0.75rem', borderRadius: '4px', marginBottom: '1rem' }}>
          {error}
        </div>
      )}

      {isLoading ? (
        <div style={{ textAlign: 'center', padding: '2rem', color: '#6b7280' }}>Loading...</div>
      ) : (
        <TenantList tenants={organizations} onAction={handleTenantAction} onRowClick={(id) => navigate(`/platform/tenants/${id}`)} />
      )}

      {showCreateModal && (
        <CreateTenantModal onClose={() => setShowCreateModal(false)} onSuccess={handleTenantCreated} />
      )}
    </div>
  );
}
