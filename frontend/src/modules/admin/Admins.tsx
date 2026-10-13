import { useState, useEffect } from 'react';
import { platformAdmins } from '../../lib/platform-api';
import type { PlatformAdmin, CreateAdminInput } from '../../lib/platform-api';
import { apiErrorMessage } from '../../lib/api';
import { formatLocalDate } from '../../lib/dateUtils';

export default function Admins() {
  const [admins, setAdmins] = useState<PlatformAdmin[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState('');
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [formData, setFormData] = useState<CreateAdminInput>({
    email: '',
    display_name: '',
    password: '',
  });

  const fetchAdmins = async () => {
    try {
      setIsLoading(true);
      setError('');
      const data = await platformAdmins.list();
      setAdmins(data);
    } catch (err: any) {
      setError(apiErrorMessage(err, 'Failed to fetch admins'));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchAdmins();
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await platformAdmins.create(formData);
      setShowCreateModal(false);
      setFormData({ email: '', display_name: '', password: '' });
      fetchAdmins();
    } catch (err: any) {
      alert(apiErrorMessage(err, 'Failed to create admin'));
    }
  };

  const handleSuspend = async (id: number) => {
    if (!confirm('Are you sure you want to suspend this admin?')) return;
    try {
      await platformAdmins.suspend(id);
      fetchAdmins();
    } catch (err: any) {
      alert(apiErrorMessage(err, 'Failed to suspend admin'));
    }
  };

  return (
    <div className="extracted-style-f334ecde23">
      <div className="extracted-style-b1a4ecd32e">
        <div>
          <h1 className="extracted-style-56f51986ef">Platform Admins</h1>
          <p className="extracted-style-5a784ab0d4">Manage platform administrators</p>
        </div>
        <button
          onClick={() => setShowCreateModal(true)}
          className="extracted-style-0eec6efe78"
        >
          Create Admin
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
        <div className="extracted-style-40736e307b">
          <table className="extracted-style-a55f31db10">
            <thead>
              <tr className="extracted-style-e5046e59fb">
                <th className="extracted-style-ea59bf4a41">ID</th>
                <th className="extracted-style-ea59bf4a41">Email</th>
                <th className="extracted-style-ea59bf4a41">Name</th>
                <th className="extracted-style-ea59bf4a41">Status</th>
                <th className="extracted-style-ea59bf4a41">Last Login</th>
                <th className="extracted-style-ea59bf4a41">Created</th>
                <th className="extracted-style-105135d318">Actions</th>
              </tr>
            </thead>
            <tbody>
              {admins.map((admin) => (
                <tr key={admin.id} className="extracted-style-62e318a01e">
                  <td className="extracted-style-bed1b6355f">{admin.id}</td>
                  <td className="extracted-style-bed1b6355f">{admin.email}</td>
                  <td className="extracted-style-bed1b6355f">{admin.display_name}</td>
                  <td className="extracted-style-6aa1ccdbf9">
                    <span className={`legacy-status-pill ${admin.is_active ? "legacy-status-active" : "legacy-status-disabled"}`}>
                      {admin.is_active ? 'Active' : 'Suspended'}
                    </span>
                  </td>
                  <td className="extracted-style-c613d744ac">
                    {admin.last_login_at ? formatLocalDate(admin.last_login_at) : 'Never'}
                  </td>
                  <td className="extracted-style-c613d744ac">
                    {formatLocalDate(admin.created_at)}
                  </td>
                  <td className="extracted-style-b15f608197">
                    {admin.is_active && (
                      <button
                        onClick={() => handleSuspend(admin.id)}
                        className="extracted-style-ba1523cd2f"
                      >
                        Suspend
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {showCreateModal && (
        <div className="extracted-style-d9bbc1b905">
          <div className="extracted-style-dc49dfe1a1">
            <h2 className="extracted-style-0662675913">Create Admin</h2>
            <form onSubmit={handleCreate}>
              <div className="extracted-style-363cdaa22c">
                <label className="extracted-style-fa7d82998f">Email</label>
                <input
                  type="email"
                  value={formData.email}
                  onChange={(e) => setFormData({ ...formData, email: e.target.value })}
                  required
                  className="extracted-style-ddcfa2b1f1"
                />
              </div>
              <div className="extracted-style-363cdaa22c">
                <label className="extracted-style-fa7d82998f">Display Name</label>
                <input
                  type="text"
                  value={formData.display_name}
                  onChange={(e) => setFormData({ ...formData, display_name: e.target.value })}
                  required
                  className="extracted-style-ddcfa2b1f1"
                />
              </div>
              <div className="extracted-style-561f378bf0">
                <label className="extracted-style-fa7d82998f">Password (min 14 chars)</label>
                <input
                  type="password"
                  value={formData.password}
                  onChange={(e) => setFormData({ ...formData, password: e.target.value })}
                  required
                  minLength={14}
                  className="extracted-style-ddcfa2b1f1"
                />
              </div>
              <div className="extracted-style-64002f3605">
                <button
                  type="button"
                  onClick={() => setShowCreateModal(false)}
                  className="extracted-style-81a373407c"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="extracted-style-deb2c21285"
                >
                  Create
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
