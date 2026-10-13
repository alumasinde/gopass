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
    <div style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, backgroundColor: 'rgba(0,0,0,0.5)', display: 'flex', justifyContent: 'center', alignItems: 'center', zIndex: 1000 }}>
      <div style={{ backgroundColor: 'white', borderRadius: '8px', width: '100%', maxWidth: '500px', maxHeight: '90vh', overflowY: 'auto', padding: '2rem' }}>
        <h2 style={{ fontSize: '1.25rem', fontWeight: 'bold', marginBottom: '1.5rem' }}>Create New Tenant</h2>

        {error && (
          <div style={{ backgroundColor: '#fee', color: '#c33', padding: '0.75rem', borderRadius: '4px', marginBottom: '1rem', fontSize: '0.875rem' }}>
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit}>
          <div style={{ marginBottom: '1rem' }}>
            <label style={{ display: 'block', marginBottom: '0.5rem', fontSize: '0.875rem', fontWeight: '500' }}>
              Organization Name *
            </label>
            <input
              type="text"
              value={formData.name}
              onChange={handleChange('name')}
              required
              style={{ width: '100%', padding: '0.5rem', border: '1px solid #ddd', borderRadius: '4px', fontSize: '1rem' }}
              placeholder="Acme Corporation"
            />
          </div>

          <div style={{ marginBottom: '1rem' }}>
            <label style={{ display: 'block', marginBottom: '0.5rem', fontSize: '0.875rem', fontWeight: '500' }}>
              Slug *
            </label>
            <input
              type="text"
              value={formData.slug}
              onChange={handleChange('slug')}
              required
              pattern="^[a-z0-9]+(?:-[a-z0-9]+)*$"
              style={{ width: '100%', padding: '0.5rem', border: '1px solid #ddd', borderRadius: '4px', fontSize: '1rem' }}
              placeholder="acme-corp"
            />
            <p style={{ fontSize: '0.75rem', color: '#6b7280', marginTop: '0.25rem' }}>Lowercase letters, numbers, and hyphens only</p>
          </div>

          <div style={{ marginBottom: '1rem' }}>
            <label style={{ display: 'block', marginBottom: '0.5rem', fontSize: '0.875rem', fontWeight: '500' }}>
              Database Mode
            </label>
            <select
              value={formData.database_mode}
              onChange={handleChange('database_mode')}
              style={{ width: '100%', padding: '0.5rem', border: '1px solid #ddd', borderRadius: '4px', fontSize: '1rem' }}
            >
              <option value="SHARED">Shared</option>
              <option value="DEDICATED">Dedicated</option>
            </select>
          </div>

          <div style={{ borderTop: '1px solid #e5e7eb', paddingTop: '1rem', marginTop: '1rem', marginBottom: '1rem' }}>
            <h3 style={{ fontSize: '1rem', fontWeight: '600', marginBottom: '1rem' }}>Admin Account</h3>

            <div style={{ marginBottom: '1rem' }}>
              <label style={{ display: 'block', marginBottom: '0.5rem', fontSize: '0.875rem', fontWeight: '500' }}>
                First Name *
              </label>
              <input
                type="text"
                value={formData.admin_first_name}
                onChange={handleChange('admin_first_name')}
                required
                style={{ width: '100%', padding: '0.5rem', border: '1px solid #ddd', borderRadius: '4px', fontSize: '1rem' }}
                placeholder="John"
              />
            </div>

            <div style={{ marginBottom: '1rem' }}>
              <label style={{ display: 'block', marginBottom: '0.5rem', fontSize: '0.875rem', fontWeight: '500' }}>
                Last Name *
              </label>
              <input
                type="text"
                value={formData.admin_last_name}
                onChange={handleChange('admin_last_name')}
                required
                style={{ width: '100%', padding: '0.5rem', border: '1px solid #ddd', borderRadius: '4px', fontSize: '1rem' }}
                placeholder="Doe"
              />
            </div>

            <div style={{ marginBottom: '1rem' }}>
              <label style={{ display: 'block', marginBottom: '0.5rem', fontSize: '0.875rem', fontWeight: '500' }}>
                Email *
              </label>
              <input
                type="email"
                value={formData.admin_email}
                onChange={handleChange('admin_email')}
                required
                style={{ width: '100%', padding: '0.5rem', border: '1px solid #ddd', borderRadius: '4px', fontSize: '1rem' }}
                placeholder="admin@acme.com"
              />
            </div>

            <div style={{ marginBottom: '1.5rem' }}>
              <label style={{ display: 'block', marginBottom: '0.5rem', fontSize: '0.875rem', fontWeight: '500' }}>
                Password *
              </label>
              <input
                type="password"
                value={formData.admin_password}
                onChange={handleChange('admin_password')}
                required
                minLength={14}
                style={{ width: '100%', padding: '0.5rem', border: '1px solid #ddd', borderRadius: '4px', fontSize: '1rem' }}
                placeholder="••••••••••••••"
              />
              <p style={{ fontSize: '0.75rem', color: '#6b7280', marginTop: '0.25rem' }}>Minimum 14 characters</p>
            </div>
          </div>

          <div style={{ display: 'flex', gap: '1rem', justifyContent: 'flex-end' }}>
            <button
              type="button"
              onClick={onClose}
              disabled={isLoading}
              style={{
                padding: '0.5rem 1rem',
                backgroundColor: 'white',
                color: '#374151',
                border: '1px solid #d1d5db',
                borderRadius: '4px',
                fontSize: '0.875rem',
                fontWeight: '500',
                cursor: isLoading ? 'not-allowed' : 'pointer',
              }}
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isLoading}
              style={{
                padding: '0.5rem 1rem',
                backgroundColor: isLoading ? '#ccc' : '#2563eb',
                color: 'white',
                border: 'none',
                borderRadius: '4px',
                fontSize: '0.875rem',
                fontWeight: '500',
                cursor: isLoading ? 'not-allowed' : 'pointer',
              }}
            >
              {isLoading ? 'Creating...' : 'Create Tenant'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
