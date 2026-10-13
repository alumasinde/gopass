import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { platformOrganizations } from '../../lib/platform-api';
import type { Organization } from '../../lib/platform-api';
import TenantList from '../../components/TenantList';
import CreateTenantModal from '../../components/CreateTenantModal';
import { apiErrorMessage } from '../../lib/api';

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
    <div className="extracted-style-f334ecde23">
      <div className="extracted-style-b1a4ecd32e">
        <div>
          <h1 className="extracted-style-56f51986ef">Tenants</h1>
          <p className="extracted-style-5a784ab0d4">Manage tenant organizations</p>
        </div>
        <button
          onClick={() => setShowCreateModal(true)}
          className="extracted-style-0eec6efe78"
        >
          Create Tenant
        </button>
      </div>

      {error && (
        <div className="extracted-style-fda1fad4e0">
          {error}
        </div>
      )}

      {isLoading ? (
        <div className="extracted-style-d322d56100">Loading...</div>
      ) : (
        <TenantList tenants={organizations} onAction={handleTenantAction} onRowClick={(id) => navigate(`/platform/tenants/${id}`)} />
      )}

      {showCreateModal && (
        <CreateTenantModal onClose={() => setShowCreateModal(false)} onSuccess={handleTenantCreated} />
      )}
    </div>
  );
}
