import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { platformOrganizations } from '../../lib/platform-api';
import type { Organization } from '../../lib/platform-api';
import { apiErrorMessage } from '../../lib/api';

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
    <div className="extracted-style-f334ecde23">
      <div className="extracted-style-c0c75bd042">
        <h1 className="extracted-style-56f51986ef">Dashboard</h1>
        <p className="extracted-style-5a784ab0d4">Platform Admin Overview</p>
      </div>

      {error && (
        <div className="extracted-style-fda1fad4e0">
          {error}
        </div>
      )}

      {isLoading ? (
        <div className="extracted-style-d322d56100">Loading...</div>
      ) : (
        <div className="extracted-style-767dc49779">
          <div className="extracted-style-79442ed173">
            <p className="extracted-style-e0797e1ebb">Total Tenants</p>
            <p className="extracted-style-932f9987a7">{organizations.length}</p>
          </div>
          <div className="extracted-style-79442ed173">
            <p className="extracted-style-e0797e1ebb">Active Tenants</p>
            <p className="extracted-style-89db87ad7c">
              {organizations.filter(o => o.is_active && o.lifecycle_status === 'ACTIVE').length}
            </p>
          </div>
          <div className="extracted-style-79442ed173">
            <p className="extracted-style-e0797e1ebb">Provisioning</p>
            <p className="extracted-style-cb110f780f">
              {organizations.filter(o => o.provisioning_status === 'PENDING').length}
            </p>
          </div>
          <div className="extracted-style-79442ed173">
            <p className="extracted-style-e0797e1ebb">Failed</p>
            <p className="extracted-style-a665333b6e">
              {organizations.filter(o => o.provisioning_status === 'FAILED').length}
            </p>
          </div>
        </div>
      )}

      <div className="extracted-style-dcb8591acd">
        <div className="extracted-style-70c54dae2e">
          <h2 className="extracted-style-e5d3d34ead">Recent Tenants</h2>
          <button
            onClick={() => navigate('/platform/tenants')}
            className="extracted-style-0eec6efe78"
          >
            View All
          </button>
        </div>
        {!isLoading && organizations.length > 0 ? (
          <div className="extracted-style-0ddf068bfb">
            <table className="extracted-style-a55f31db10">
              <thead>
                <tr className="extracted-style-e5046e59fb">
                  <th className="extracted-style-ea59bf4a41">Name</th>
                  <th className="extracted-style-ea59bf4a41">Status</th>
                  <th className="extracted-style-ea59bf4a41">Created</th>
                </tr>
              </thead>
              <tbody>
                {organizations.slice(0, 5).map((tenant) => (
                  <tr key={tenant.id} className="extracted-style-6422313b32" onClick={() => navigate(`/platform/tenants/${tenant.id}`)}>
                    <td className="extracted-style-29797852fe">{tenant.name}</td>
                    <td className="extracted-style-6aa1ccdbf9">
                      <span className={`legacy-status-pill ${tenant.lifecycle_status === "ACTIVE" ? "legacy-status-active" : "legacy-status-pending"}`}>
                        {tenant.lifecycle_status}
                      </span>
                    </td>
                    <td className="extracted-style-c613d744ac">
                      {new Date(tenant.created_at).toLocaleDateString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="extracted-style-05b7259ef9">No tenants yet</p>
        )}
      </div>
    </div>
  );
}
