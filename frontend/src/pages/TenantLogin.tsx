import { useEffect, useState } from 'react';
import type { FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useTenantAuth } from '../contexts/TenantAuthContext';
import { tenantApiErrorMessage, tenantAuthApi } from '../lib/tenant-api';
import { tenantSlugFromHostname, isTenantHostname } from '../lib/tenant-host';

export default function TenantLogin() {
  const host = window.location.hostname.toLowerCase();
  const hostSlug = tenantSlugFromHostname(host);
  const isTenantHost = isTenantHostname(host);
  const [organizationSlug, setOrganizationSlug] = useState(hostSlug || (isTenantHost ? '' : localStorage.getItem('tenant_organization_slug') || ''));
  const [resolvedOrganization, setResolvedOrganization] = useState('');
  const [resolvingHost, setResolvingHost] = useState(false);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const { login } = useTenantAuth();
  const navigate = useNavigate();

  useEffect(() => {
    // Slug subdomains are self-describing. Other tenant domains must be registered
    // and verified in the platform control plane before they can be used to log in.
    if (!isTenantHost || hostSlug || host === 'localhost' || host === '127.0.0.1') return;
    let cancelled = false;
    setResolvingHost(true);
    tenantAuthApi.resolveHost(host)
      .then((result) => {
        if (cancelled) return;
        setOrganizationSlug(result.slug);
        setResolvedOrganization(result.name);
        setError('');
      })
      .catch((err) => {
        if (!cancelled) setError(tenantApiErrorMessage(err, 'This domain is not connected to a GoPass organization yet.'));
      })
      .finally(() => { if (!cancelled) setResolvingHost(false); });
    return () => { cancelled = true; };
  }, [host, hostSlug, isTenantHost]);

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError('');
    setSubmitting(true);
    try {
      const effectiveSlug = hostSlug || organizationSlug;
      if (!effectiveSlug.trim()) throw new Error('Enter your organization slug or open your organization URL.');
      await login(effectiveSlug, email, password);
      navigate('/tenant/dashboard', { replace: true });
    } catch (err) {
      setError(tenantApiErrorMessage(err, 'Sign in failed. Check your organization, email and password.'));
    } finally {
      setSubmitting(false);
    }
  };

  const showSlugInput = !hostSlug && (!isTenantHost || host === 'localhost' || host === '127.0.0.1');

  return (
    <main className="tenant-auth-page">
      <section className="tenant-auth-card">
        <div className="tenant-auth-brand"><span className="tenant-brand-mark">G</span><div><strong>GoPass</strong><small>Visitor & gatepass management</small></div></div>
        <div className="tenant-auth-heading">
          <span className="eyebrow">TENANT WORKSPACE</span>
          <h1>Welcome back</h1>
          <p>Sign in to manage visitors, gatepasses and approvals for your organization.</p>
          {isTenantHost && <div className="tenant-host-chip"><span className="tenant-live-dot" /> {resolvedOrganization || organizationSlug || host}{resolvingHost ? ' · checking domain…' : ''}</div>}
        </div>
        {error && <div className="tenant-alert tenant-alert-error" role="alert">{error}</div>}
        <form onSubmit={submit} className="tenant-form">
          {showSlugInput && <>
            <label htmlFor="organizationSlug">Organization slug</label>
            <input id="organizationSlug" autoComplete="organization" value={organizationSlug} onChange={(event) => setOrganizationSlug(event.target.value.toLowerCase().replace(/\s+/g, '-'))} placeholder="e.g. acme-security" required minLength={2} />
          </>}
          <label htmlFor="tenantEmail">Email address</label>
          <input id="tenantEmail" type="email" autoComplete="username" value={email} onChange={(event) => setEmail(event.target.value)} placeholder="you@company.com" required />
          <label htmlFor="tenantPassword">Password</label>
          <input id="tenantPassword" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="Enter your password" required />
          <button className="tenant-primary-button" type="submit" disabled={submitting || resolvingHost || (isTenantHost && !organizationSlug)}>
            {submitting ? <><span className="tenant-spinner" /> Signing in…</> : resolvingHost ? 'Checking organization…' : 'Sign in to workspace'}
          </button>
        </form>
        <div className="tenant-auth-footer"><span>Access is restricted to your organization.</span><Link to="/platform/login">Platform Admin</Link></div>
      </section>
      <aside className="tenant-auth-aside"><div className="tenant-aside-orb orb-one" /><div className="tenant-aside-orb orb-two" /><div className="tenant-aside-content"><span className="tenant-aside-tag">CONTROL YOUR FRONT GATE</span><h2>Every visit.<br />Accounted for.</h2><p>Bring visitor registration, approvals and gate activity into one clear workspace.</p><div className="tenant-aside-points"><span><i>✓</i> Visitor visibility</span><span><i>✓</i> Approval workflows</span><span><i>✓</i> Traceable gate activity</span></div></div><div className="tenant-aside-bottom">GoPass · Secure visitor operations</div></aside>
    </main>
  );
}
