import { useState, useEffect } from 'react';
import type { FormEvent } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { platformOrganizations, platformTenantDomains } from '../../lib/platform-api';
import type { Organization, TenantDomain } from '../../lib/platform-api';
import { formatLocalDate } from '../../lib/dateUtils';
import { apiErrorMessage } from '../../lib/api';

export default function TenantDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [tenant, setTenant] = useState<Organization | null>(null);
  const [editingIdentity, setEditingIdentity] = useState(false);
  const [identityForm, setIdentityForm] = useState({name:'',slug:''});
  const [identitySaving, setIdentitySaving] = useState(false);
  const [identityError, setIdentityError] = useState('');
  const [identityMessage, setIdentityMessage] = useState('');
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState('');
  const [databaseDSN, setDatabaseDSN] = useState('');
  const [savingDatabase, setSavingDatabase] = useState(false);
  const [databaseMessage, setDatabaseMessage] = useState('');
  const [databaseError, setDatabaseError] = useState('');
  const [domains, setDomains] = useState<TenantDomain[]>([]);
  const [domainInput, setDomainInput] = useState('');
  const [domainLoading, setDomainLoading] = useState(false);
  const [domainError, setDomainError] = useState('');
  const [domainMessage, setDomainMessage] = useState('');
  const [newDomainChallenge, setNewDomainChallenge] = useState<TenantDomain | null>(null);

  useEffect(() => {
    const fetchTenant = async () => {
      if (!id) return;
      try {
        setIsLoading(true);
        setError('');
        const data = await platformOrganizations.get(Number(id));
        setTenant(data);
        setIdentityForm({name:data.name,slug:data.slug});
      } catch (err: any) {
        setError(apiErrorMessage(err, 'Failed to fetch tenant details'));
      } finally {
        setIsLoading(false);
      }
    };

    fetchTenant();
  }, [id]);

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    platformTenantDomains.list(Number(id))
      .then((items) => { if (!cancelled) setDomains(Array.isArray(items) ? items : []); })
      .catch((err) => { if (!cancelled) setDomainError(apiErrorMessage(err, 'Unable to load tenant domains. Apply platform migrations first if this is a new setup.')); });
    return () => { cancelled = true; };
  }, [id]);

  const handleSaveIdentity = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault(); if(!tenant)return; setIdentitySaving(true);setIdentityError('');setIdentityMessage('');
    try { const updated=await platformOrganizations.update(tenant.id,{name:identityForm.name.trim(),slug:identityForm.slug.trim().toLowerCase()});setTenant(current=>current?{...current,...updated}:updated);setIdentityForm({name:updated.name,slug:updated.slug});setEditingIdentity(false);setIdentityMessage('Tenant details updated.'); }
    catch(err:any){setIdentityError(apiErrorMessage(err,'Tenant details could not be updated.'));}finally{setIdentitySaving(false)}
  };

  const handleAddDomain = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!tenant || !domainInput.trim()) return;
    setDomainLoading(true); setDomainError(''); setDomainMessage(''); setNewDomainChallenge(null);
    try {
      const created = await platformTenantDomains.create(tenant.id, domainInput.trim());
      setDomainInput('');
      setNewDomainChallenge(created);
      setDomainMessage('Domain registered. Add the TXT verification record below, then verify it.');
      setDomains((current) => [created, ...current.filter((item) => item.id !== created.id)]);
    } catch (err: any) {
      setDomainError(apiErrorMessage(err, 'Could not register domain.'));
    } finally { setDomainLoading(false); }
  };

  const handleVerifyDomain = async (domainID: number) => {
    if (!tenant) return;
    setDomainLoading(true); setDomainError(''); setDomainMessage('');
    try {
      const updated = await platformTenantDomains.verify(tenant.id, domainID);
      setDomains((current) => current.map((item) => item.id === domainID ? { ...item, ...updated } : item));
      setDomainMessage(`${updated.domain} is verified and active.`);
      setNewDomainChallenge(null);
    } catch (err: any) {
      setDomainError(apiErrorMessage(err, 'DNS verification failed. Confirm the TXT record has propagated.'));
    } finally { setDomainLoading(false); }
  };

  const handleSuspend = async () => {
    if (!tenant || !confirm('Are you sure you want to suspend this tenant?')) return;
    try {
      await platformOrganizations.suspend(tenant.id);
      const updated = await platformOrganizations.get(tenant.id);
      setTenant(updated);
    } catch (err: any) {
      alert(apiErrorMessage(err, 'Failed to suspend tenant'));
    }
  };

  const handleResume = async () => {
    if (!tenant || !confirm('Are you sure you want to resume this tenant?')) return;
    try {
      await platformOrganizations.resume(tenant.id);
      const updated = await platformOrganizations.get(tenant.id);
      setTenant(updated);
    } catch (err: any) {
      alert(apiErrorMessage(err, 'Failed to resume tenant'));
    }
  };

  const handleDeprovision = async () => {
    if (!tenant || !confirm('Are you sure you want to deprovision this tenant? This action cannot be undone.')) return;
    try {
      await platformOrganizations.deprovision(tenant.id);
      const updated = await platformOrganizations.get(tenant.id);
      setTenant(updated);
    } catch (err: any) {
      alert(apiErrorMessage(err, 'Failed to deprovision tenant'));
    }
  };

  const handleRetryProvisioning = async () => {
    if (!tenant) return;
    try {
      const result = await platformOrganizations.retryProvisioning(tenant.id);
      if (result.id !== tenant.id) {
        navigate(`/platform/tenants/${result.id}`, { replace: true });
        const updated = await platformOrganizations.get(result.id);
        setTenant(updated);
      } else {
        const updated = await platformOrganizations.get(tenant.id);
        setTenant(updated);
      }
    } catch (err: any) {
      alert(apiErrorMessage(err, 'Failed to retry provisioning'));
    }
  };

  const handleSaveDatabase = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!tenant || !databaseDSN.trim()) return;
    setSavingDatabase(true);
    setDatabaseError('');
    setDatabaseMessage('');
    try {
      await platformOrganizations.setDatabaseConfig(tenant.id, databaseDSN.trim());
      setDatabaseDSN('');
      setDatabaseMessage('Database credentials validated and stored encrypted. Provisioning will run in the background.');
      const updated = await platformOrganizations.get(tenant.id);
      setTenant(updated);
    } catch (err: any) {
      setDatabaseError(apiErrorMessage(err, 'Failed to configure the dedicated database'));
    } finally {
      setSavingDatabase(false);
    }
  };

  if (isLoading) {
    return (
      <div className="extracted-style-bb36163196">
        Loading...
      </div>
    );
  }

  if (error || !tenant) {
    return (
      <div className="extracted-style-42aa6d9c6f">
        <div className="extracted-style-fda1fad4e0">
          {error || 'Tenant not found'}
        </div>
        <button
          onClick={() => navigate('/platform/tenants')}
          className="extracted-style-1b0a2057a2"
        >
          Back to Tenants
        </button>
      </div>
    );
  }



  return (
    <div className="extracted-style-f334ecde23">
      <div className="extracted-style-c0c75bd042">
        <button
          onClick={() => navigate('/platform/tenants')}
          className="extracted-style-31bc187688"
        >
          ← Back to Tenants
        </button>
        <h1 className="extracted-style-56f51986ef">{tenant.name}</h1>
        <p className="extracted-style-5a784ab0d4">Slug: {tenant.slug}</p>
      </div>

      <section className="crud-panel crud-tenant-identity">
        <div className="crud-toolbar"><div><h2>Tenant identity</h2><p>Update the organization name or slug used in tenant URLs.</p></div>{!editingIdentity&&tenant.lifecycle_status==='ACTIVE'&&tenant.provisioning_status==='READY'&&<button type="button" className="crud-button crud-button-muted" onClick={()=>setEditingIdentity(true)}>Edit details</button>}</div>
        {identityMessage&&<div className="crud-notice">{identityMessage}</div>}{identityError&&<div className="crud-inline-error" role="alert">{identityError}</div>}
        {editingIdentity&&<form className="crud-settings-form" onSubmit={handleSaveIdentity}><label className="crud-field"><span>Organization name *</span><input required value={identityForm.name} onChange={e=>setIdentityForm(v=>({...v,name:e.target.value}))}/></label><label className="crud-field"><span>Tenant slug *</span><input required pattern="[a-z0-9]+(?:-[a-z0-9]+)*" value={identityForm.slug} onChange={e=>setIdentityForm(v=>({...v,slug:e.target.value.toLowerCase()}))}/><small>Changing this changes the tenant's local hostname and login URL.</small></label><footer className="crud-modal-footer"><button type="button" className="crud-button crud-button-muted" onClick={()=>{setEditingIdentity(false);setIdentityForm({name:tenant.name,slug:tenant.slug});}} disabled={identitySaving}>Cancel</button><button className="crud-button crud-button-primary" disabled={identitySaving}>{identitySaving?'Saving…':'Save details'}</button></footer></form>}
      </section>

      <div className="extracted-style-ba72f8a298">
        <div className="extracted-style-79442ed173">
          <h3 className="extracted-style-2135ac1de0">Lifecycle Status</h3>
          <div className={`legacy-status-pill legacy-status-${tenant.lifecycle_status.toLowerCase()}`}>
            {tenant.lifecycle_status}
          </div>
        </div>

        <div className="extracted-style-79442ed173">
          <h3 className="extracted-style-2135ac1de0">Provisioning Status</h3>
          <div className={`legacy-status-pill legacy-provisioning-${tenant.provisioning_status.toLowerCase()}`}>
            {tenant.provisioning_status}
          </div>
          {tenant.provisioning_error && (
            <p className="extracted-style-ec2911d28b">{tenant.provisioning_error}</p>
          )}
        </div>

        <div className="extracted-style-79442ed173">
          <h3 className="extracted-style-2135ac1de0">Database Mode</h3>
          <p className="extracted-style-86cadcdf01">{tenant.database_mode}</p>
        </div>
      </div>

      {tenant.database_mode === 'DEDICATED' && (!tenant.database_configured || tenant.provisioning_status === 'FAILED' || tenant.provisioning_status === 'BLOCKED') && tenant.lifecycle_status !== 'DEPROVISIONED' && (
        <div className="extracted-style-2841286d86">
          <h3 className="extracted-style-17b318351d">Dedicated database configuration</h3>
          <p className="extracted-style-57f011e7e9">Enter a MySQL DSN for an empty database or an existing GoPass tenant schema. Credentials are encrypted before they are stored. Do not use the platform or shared tenant database.</p>
          {databaseError && <div role="alert" className="extracted-style-4a79cfe8ae">{databaseError}</div>}
          {databaseMessage && <div role="status" className="extracted-style-09b37f581d">{databaseMessage}</div>}
          <form onSubmit={handleSaveDatabase}>
            <label htmlFor="tenant-database-dsn" className="extracted-style-910ede2417">MySQL DSN</label>
            <input id="tenant-database-dsn" type="password" autoComplete="new-password" value={databaseDSN} onChange={event => setDatabaseDSN(event.target.value)} required placeholder="user:password@tcp(host:3306)/tenant_db?parseTime=true" className="extracted-style-a640830f73" />
            <button type="submit" disabled={savingDatabase} className="legacy-action-button legacy-db-submit-button">{savingDatabase ? 'Validating…' : 'Save and provision database'}</button>
          </form>
        </div>
      )}

      <section className="extracted-style-2841286d86">
        <h3 className="extracted-style-7f1ae1ed9b">Tenant URLs & custom domains</h3>
        <p className="extracted-style-916e0bc14b">Local development URL: <code>http://{tenant.slug}.localhost:5173/tenant/login</code>. For a production custom domain, point DNS to your GoPass frontend and verify ownership with a TXT record.</p>
        {domainError && <div role="alert" className="extracted-style-f31a989fa5">{domainError}</div>}
        {domainMessage && <div role="status" className="extracted-style-5b9024d508">{domainMessage}</div>}
        <form onSubmit={handleAddDomain} className="extracted-style-a4db9c1403">
          <input aria-label="Custom domain" value={domainInput} onChange={(event) => setDomainInput(event.target.value)} placeholder="visitors.example.com" required className="extracted-style-95b3dc61ca" />
          <button type="submit" disabled={domainLoading} className="extracted-style-676c03848a">{domainLoading ? 'Saving…' : 'Add domain'}</button>
        </form>
        {newDomainChallenge?.verification_token && <div className="extracted-style-22109adbc8">
          <strong>DNS verification record</strong>
          <p className="extracted-style-58d891e89f">Create this TXT record at your DNS provider. Wait for DNS propagation, then click Verify.</p>
          <p><small>Host / Name</small><br /><code>{newDomainChallenge.verification_host || `_gopass-verification.${newDomainChallenge.domain}`}</code></p>
          <p><small>TXT value</small><br /><code className="extracted-style-115422ed29">{newDomainChallenge.verification_token}</code></p>
          <button type="button" onClick={() => void handleVerifyDomain(newDomainChallenge.id)} disabled={domainLoading} className="extracted-style-725a8b9746">Verify DNS now</button>
        </div>}
        {domains.length === 0 ? <p className="extracted-style-2b9f58dcaa">No custom domains registered.</p> : <div className="extracted-style-816aace52c">
          {domains.map((domain) => <div key={domain.id} className="extracted-style-883a466694">
            <div><strong>{domain.domain}</strong><div className="extracted-style-925122e854">{domain.is_verified && domain.is_active ? 'Verified and active' : 'Pending DNS verification'}</div></div>
            {!domain.is_verified && <button type="button" onClick={() => setNewDomainChallenge(domain)} className="extracted-style-1e4c7369e5">Show verification</button>}
            {!domain.is_verified && newDomainChallenge?.id === domain.id && <button type="button" onClick={() => void handleVerifyDomain(domain.id)} disabled={domainLoading} className="extracted-style-a670abc952">Verify DNS</button>}
          </div>)}
        </div>}
      </section>

      <div className="extracted-style-2841286d86">
        <h3 className="extracted-style-2135ac1de0">Timeline</h3>
        <div className="extracted-style-a7b25ee1f6">
          <div>
            <p className="extracted-style-fa1a143eee">Created</p>
            <p className="extracted-style-245ff4a0c3">{formatLocalDate(tenant.created_at)}</p>
          </div>
          <div>
            <p className="extracted-style-fa1a143eee">Updated</p>
            <p className="extracted-style-245ff4a0c3">{formatLocalDate(tenant.updated_at)}</p>
          </div>
          {tenant.provisioned_at && (
            <div>
              <p className="extracted-style-fa1a143eee">Provisioned</p>
              <p className="extracted-style-245ff4a0c3">{formatLocalDate(tenant.provisioned_at)}</p>
            </div>
          )}
          {tenant.suspended_at && (
            <div>
              <p className="extracted-style-fa1a143eee">Suspended</p>
              <p className="extracted-style-245ff4a0c3">{formatLocalDate(tenant.suspended_at)}</p>
            </div>
          )}
        </div>
      </div>

      <div className="extracted-style-79442ed173">
        <h3 className="extracted-style-2135ac1de0">Actions</h3>
        <div className="extracted-style-c1ddbea1d6">
          {tenant.lifecycle_status === 'ACTIVE' && (
            <button
              onClick={handleSuspend}
              className="extracted-style-7529b5c114"
            >
              Suspend
            </button>
          )}
          {tenant.lifecycle_status === 'SUSPENDED' && (
            <button
              onClick={handleResume}
              className="extracted-style-8ac5e9225f"
            >
              Resume
            </button>
          )}
          {(tenant.lifecycle_status === 'ACTIVE' || tenant.lifecycle_status === 'SUSPENDED') && (
            <button
              onClick={handleDeprovision}
              className="extracted-style-1fdb1fc711"
            >
              Deprovision
            </button>
          )}
          {(tenant.provisioning_status === 'FAILED' || tenant.provisioning_status === 'BLOCKED') && (
            <button
              onClick={handleRetryProvisioning}
              className="extracted-style-78ba4a4e8e"
            >
              Retry Provisioning
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
