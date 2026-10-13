import { useState } from 'react';
import type { ReactNode } from 'react';
import { NavLink, useNavigate } from 'react-router-dom';
import { useTenantAuth } from '../contexts/TenantAuthContext';

export default function TenantLayout({ children }: { children: ReactNode }) {
  const { user, organization, logout } = useTenantAuth();
  const [signingOut, setSigningOut] = useState(false);
  const navigate = useNavigate();
  const permissions = user?.permissions ?? [];
  const canAny = (...codes: string[]) => permissions.some((code) => codes.includes(code));
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
          {canAny('visitors.view') && <NavLink to="/tenant/visitors" className={({ isActive }) => `tenant-nav-item${isActive ? ' active' : ''}`}><span>♙</span> Visitors</NavLink>}
          {canAny('gatepasses.view') && <NavLink to="/tenant/visits" className={({ isActive }) => `tenant-nav-item${isActive ? ' active' : ''}`}><span>◷</span> Visits</NavLink>}
          {canAny('approvals.view', 'approvals.approve') && <button className="tenant-nav-item tenant-nav-disabled" type="button" title="Approval pages are not included in this UI phase" disabled><span>✓</span> Approvals <small>Next</small></button>}
          {canAny('checkins.view', 'checkins.perform', 'checkouts.view', 'checkouts.perform') && <button className="tenant-nav-item tenant-nav-disabled" type="button" title="Security desk pages are not included in this UI phase" disabled><span>⌖</span> Security desk <small>Next</small></button>}
          {canAny('users.view', 'roles.view', 'sites.view', 'gates.view', 'pass_types.view', 'organization.view', 'identification_types.view') && <div className="tenant-nav-label">ADMINISTRATION</div>}
          {canAny('users.view') && <NavLink to="/tenant/users" className={({ isActive }) => `tenant-nav-item${isActive ? ' active' : ''}`}><span>♧</span> Users</NavLink>}
          {canAny('roles.view') && <NavLink to="/tenant/roles" className={({ isActive }) => `tenant-nav-item${isActive ? ' active' : ''}`}><span>⚙</span> Roles & permissions</NavLink>}
          {canAny('sites.view') && <NavLink to="/tenant/sites" className={({ isActive }) => `tenant-nav-item${isActive ? ' active' : ''}`}><span>⌖</span> Sites</NavLink>}
          {canAny('gates.view') && <NavLink to="/tenant/gates" className={({ isActive }) => `tenant-nav-item${isActive ? ' active' : ''}`}><span>⌖</span> Gates</NavLink>}
          {canAny('pass_types.view') && <NavLink to="/tenant/pass-types" className={({ isActive }) => `tenant-nav-item${isActive ? ' active' : ''}`}><span>▤</span> Pass types</NavLink>}
          {canAny('identification_types.view') && <NavLink to="/tenant/identification-types" className={({ isActive }) => `tenant-nav-item${isActive ? ' active' : ''}`}><span>▣</span> Identification types</NavLink>}
          {canAny('organization.view') && <NavLink to="/tenant/organization" className={({ isActive }) => `tenant-nav-item${isActive ? ' active' : ''}`}><span>⚙</span> Organization settings</NavLink>}
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
