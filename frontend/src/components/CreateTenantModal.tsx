import { useState } from 'react';
import type { FormEvent } from 'react';
import { platformOrganizations } from '../lib/platform-api';
import type { CreateOrganizationInput } from '../lib/platform-api';
import { apiErrorMessage } from '../lib/api';

interface CreateTenantModalProps {
  onClose: () => void;
  onSuccess: () => void;
}

export default function CreateTenantModal({ onClose, onSuccess }: CreateTenantModalProps) {
  const [formData, setFormData] = useState<CreateOrganizationInput>({
    name: '',
    slug: '',
    database_mode: 'SHARED',
    admin_first_name: '',
    admin_last_name: '',
    admin_email: '',
    admin_password: '',
  });
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    setIsLoading(true);

    try {
      await platformOrganizations.create(formData);
      onSuccess();
    } catch (err: any) {
      setError(apiErrorMessage(err, 'Failed to create tenant'));
    } finally {
      setIsLoading(false);
    }
  };

  const handleChange = (field: keyof CreateOrganizationInput) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    setFormData(prev => ({ ...prev, [field]: e.target.value }));
  };

  return (
    <div className="extracted-style-d9bbc1b905">
      <div className="extracted-style-70153fe236">
        <h2 className="extracted-style-0662675913">Create New Tenant</h2>

        {error && (
          <div className="extracted-style-bfdca8fd79">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit}>
          <div className="extracted-style-363cdaa22c">
            <label className="extracted-style-fa7d82998f">
              Organization Name *
            </label>
            <input
              type="text"
              value={formData.name}
              onChange={handleChange('name')}
              required
              className="extracted-style-ddcfa2b1f1"
              placeholder="Acme Corporation"
            />
          </div>

          <div className="extracted-style-363cdaa22c">
            <label className="extracted-style-fa7d82998f">
              Slug *
            </label>
            <input
              type="text"
              value={formData.slug}
              onChange={handleChange('slug')}
              required
              pattern="^[a-z0-9]+(?:-[a-z0-9]+)*$"
              className="extracted-style-ddcfa2b1f1"
              placeholder="acme-corp"
            />
            <p className="extracted-style-6dfacc9407">Lowercase letters, numbers, and hyphens only</p>
          </div>

          <div className="extracted-style-363cdaa22c">
            <label className="extracted-style-fa7d82998f">
              Database Mode
            </label>
            <select
              value={formData.database_mode}
              onChange={handleChange('database_mode')}
              className="extracted-style-ddcfa2b1f1"
            >
              <option value="SHARED">Shared</option>
              <option value="DEDICATED">Dedicated</option>
            </select>
            {formData.database_mode === 'DEDICATED' && (
              <p className="extracted-style-decf3c58f4">After creating the tenant, open its details and configure a dedicated MySQL database. The tenant stays inactive until provisioning succeeds.</p>
            )}
          </div>

          <div className="extracted-style-2ebf89bc60">
            <h3 className="extracted-style-2135ac1de0">Admin Account</h3>

            <div className="extracted-style-363cdaa22c">
              <label className="extracted-style-fa7d82998f">
                First Name *
              </label>
              <input
                type="text"
                value={formData.admin_first_name}
                onChange={handleChange('admin_first_name')}
                required
                className="extracted-style-ddcfa2b1f1"
                placeholder="John"
              />
            </div>

            <div className="extracted-style-363cdaa22c">
              <label className="extracted-style-fa7d82998f">
                Last Name *
              </label>
              <input
                type="text"
                value={formData.admin_last_name}
                onChange={handleChange('admin_last_name')}
                required
                className="extracted-style-ddcfa2b1f1"
                placeholder="Doe"
              />
            </div>

            <div className="extracted-style-363cdaa22c">
              <label className="extracted-style-fa7d82998f">
                Email *
              </label>
              <input
                type="email"
                value={formData.admin_email}
                onChange={handleChange('admin_email')}
                required
                className="extracted-style-ddcfa2b1f1"
                placeholder="admin@acme.com"
              />
            </div>

            <div className="extracted-style-561f378bf0">
              <label className="extracted-style-fa7d82998f">
                Password *
              </label>
              <input
                type="password"
                value={formData.admin_password}
                onChange={handleChange('admin_password')}
                required
                minLength={14}
                className="extracted-style-ddcfa2b1f1"
                placeholder="••••••••••••••"
              />
              <p className="extracted-style-6dfacc9407">Minimum 14 characters</p>
            </div>
          </div>

          <div className="extracted-style-64002f3605">
            <button
              type="button"
              onClick={onClose}
              disabled={isLoading}
              className="legacy-action-button legacy-action-button-cancel"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isLoading}
              className="legacy-action-button legacy-action-button-submit"
            >
              {isLoading ? 'Creating...' : 'Create Tenant'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
