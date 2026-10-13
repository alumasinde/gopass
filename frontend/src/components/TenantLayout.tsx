import { useState } from 'react';
import type { ReactNode } from 'react';
import { NavLink, useNavigate } from 'react-router-dom';
import { useTenantAuth } from '../contexts/TenantAuthContext';

export default function TenantLayout({ children }: { children: ReactNode }) {
  const { user, organization, logout } = useTenantAuth();
  const [signingOut, setSigningOut] = useState(false);
  const navigate = useNavigate();
  const initials = `${user?.first_name?.[0] ?? ''}${user?.last_name?.[0] ?? ''}`.toUpperCase() || 'U';

  const signOut = async () => {
    if (signingOut) return;
    setSigningOut(true);
    await logout();
    navigate('/tenant/login', { replace: true });
  };

  return (
    <div className="tenant-shell">
      <aside className="tenant-sidebar">
        <div className="tenant-sidebar-brand"><span className="tenant-brand-mark">G</span><div><strong>GoPass</strong><small>Tenant workspace</small></div></div>
        <div className="tenant-sidebar-org"><span className="tenant-org-avatar">{(organization?.name || 'O').slice(0, 1).toUpperCase()}</span><div><strong>{organization?.name || 'Your organization'}</strong><small>{organization?.slug || 'Workspace'}</small></div></div>
        <div className="tenant-nav-label">WORKSPACE</div>
        <nav className="tenant-nav" aria-label="Tenant navigation">
          <NavLink to="/tenant/dashboard" className={({ isActive }) => `tenant-nav-item${isActive ? ' active' : ''}`}><span>▦</span> Overview</NavLink>
          <button className="tenant-nav-item tenant-nav-disabled" type="button" title="Visitor pages are the next UI phase" disabled><span>♙</span> Visitors <small>Next</small></button>
          <button className="tenant-nav-item tenant-nav-disabled" type="button" title="Gatepass pages are the next UI phase" disabled><span>▤</span> Gatepasses <small>Next</small></button>
          <button className="tenant-nav-item tenant-nav-disabled" type="button" title="Approval pages are the next UI phase" disabled><span>✓</span> Approvals <small>Next</small></button>
        </nav>
        <div className="tenant-sidebar-bottom"><div className="tenant-secure-label"><span /> Tenant session active</div><button className="tenant-signout" onClick={() => void signOut()} disabled={signingOut}><span>↪</span>{signingOut ? 'Signing out…' : 'Sign out'}</button></div>
      </aside>
      <div className="tenant-main-column">
        <header className="tenant-topbar"><div><span className="tenant-topbar-kicker">WORKSPACE</span><strong>{organization?.name || 'GoPass Dashboard'}</strong></div><div className="tenant-topbar-user"><div className="tenant-avatar">{initials}</div><div><strong>{user ? `${user.first_name} ${user.last_name}`.trim() : 'User'}</strong><span>{user?.email}</span></div></div></header>
        <main className="tenant-main-content">{children}</main>
      </div>
    </div>
  );
}
