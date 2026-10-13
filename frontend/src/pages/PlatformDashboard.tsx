import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { usePlatformAuth } from '../contexts/PlatformAuthContext';
import { platformOrganizations } from '../lib/platform-api';
import type { Organization } from '../lib/platform-api';
import TenantList from '../components/TenantList';
import CreateTenantModal from '../components/CreateTenantModal';
import { apiErrorMessage } from '../lib/api';

export default function PlatformDashboard() {
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState('');
  const [showCreateModal, setShowCreateModal] = useState(false);
  const { logout } = usePlatformAuth();
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

  const handleLogout = async () => {
    await logout();
    navigate('/platform/login');
  };

  const handleTenantCreated = () => {
    setShowCreateModal(false);
    fetchOrganizations();
  };

  const handleTenantAction = () => {
    fetchOrganizations();
  };

  return (
    <div style={{ minHeight: '100vh', backgroundColor: '#f5f5f5' }}>
      {/* Header */}
      <header style={{ backgroundColor: 'white', borderBottom: '1px solid #e5e7eb', padding: '1rem 2rem' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', maxWidth: '1200px', margin: '0 auto' }}>
          <div>
            <h1 style={{ fontSize: '1.25rem', fontWeight: 'bold', margin: 0 }}>Platform Admin Dashboard</h1>
            <p style={{ fontSize: '0.875rem', color: '#6b7280', margin: '0.25rem 0 0 0' }}>Manage tenant organizations</p>
          </div>
          <div style={{ display: 'flex', gap: '1rem', alignItems: 'center' }}>
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
            <button
              onClick={handleLogout}
              style={{
                padding: '0.5rem 1rem',
                backgroundColor: 'white',
                color: '#374151',
                border: '1px solid #d1d5db',
                borderRadius: '4px',
                fontSize: '0.875rem',
                fontWeight: '500',
                cursor: 'pointer',
              }}
            >
              Logout
            </button>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main style={{ maxWidth: '1200px', margin: '2rem auto', padding: '0 2rem' }}>
        {error && (
          <div style={{ backgroundColor: '#fee', color: '#c33', padding: '0.75rem', borderRadius: '4px', marginBottom: '1rem' }}>
            {error}
          </div>
        )}

        {isLoading ? (
          <div style={{ textAlign: 'center', padding: '2rem', color: '#6b7280' }}>Loading...</div>
        ) : (
          <TenantList tenants={organizations} onAction={handleTenantAction} />
        )}
      </main>

      {/* Create Tenant Modal */}
      {showCreateModal && (
        <CreateTenantModal onClose={() => setShowCreateModal(false)} onSuccess={handleTenantCreated} />
      )}
    </div>
  );
}
