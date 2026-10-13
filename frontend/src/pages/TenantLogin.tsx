import { useState } from 'react';
import type { FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useTenantAuth } from '../contexts/TenantAuthContext';
import { tenantApiErrorMessage } from '../lib/tenant-api';

export default function TenantLogin() {
  const [organizationSlug, setOrganizationSlug] = useState(localStorage.getItem('tenant_organization_slug') || '');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const { login } = useTenantAuth();
  const navigate = useNavigate();

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError('');
    setSubmitting(true);
    try {
      await login(organizationSlug, email, password);
      navigate('/tenant/dashboard', { replace: true });
    } catch (err) {
      setError(tenantApiErrorMessage(err, 'Sign in failed. Check your organization, email and password.'));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <main className="tenant-auth-page">
      <section className="tenant-auth-card">
        <div className="tenant-auth-brand"><span className="tenant-brand-mark">G</span><div><strong>GoPass</strong><small>Visitor & gatepass management</small></div></div>
        <div className="tenant-auth-heading"><span className="eyebrow">TENANT WORKSPACE</span><h1>Welcome back</h1><p>Sign in to manage visitors, gatepasses and approvals for your organization.</p></div>
        {error && <div className="tenant-alert tenant-alert-error" role="alert">{error}</div>}
        <form onSubmit={submit} className="tenant-form">
          <label htmlFor="organizationSlug">Organization slug</label>
          <input id="organizationSlug" autoComplete="organization" value={organizationSlug} onChange={(event) => setOrganizationSlug(event.target.value.toLowerCase().replace(/\s+/g, '-'))} placeholder="e.g. acme-security" required minLength={2} />
          <label htmlFor="tenantEmail">Email address</label>
          <input id="tenantEmail" type="email" autoComplete="username" value={email} onChange={(event) => setEmail(event.target.value)} placeholder="you@company.com" required />
          <label htmlFor="tenantPassword">Password</label>
          <input id="tenantPassword" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="Enter your password" required />
          <button className="tenant-primary-button" type="submit" disabled={submitting}>{submitting ? <><span className="tenant-spinner" /> Signing in…</> : 'Sign in to workspace'}</button>
        </form>
        <div className="tenant-auth-footer"><span>Access is restricted to your organization.</span><Link to="/platform/login">Platform Admin</Link></div>
      </section>
      <aside className="tenant-auth-aside"><div className="tenant-aside-orb orb-one" /><div className="tenant-aside-orb orb-two" /><div className="tenant-aside-content"><span className="tenant-aside-tag">CONTROL YOUR FRONT GATE</span><h2>Every visit.<br />Accounted for.</h2><p>Bring visitor registration, approvals and gate activity into one clear workspace.</p><div className="tenant-aside-points"><span><i>✓</i> Visitor visibility</span><span><i>✓</i> Approval workflows</span><span><i>✓</i> Traceable gate activity</span></div></div><div className="tenant-aside-bottom">GoPass · Secure visitor operations</div></aside>
    </main>
  );
}
