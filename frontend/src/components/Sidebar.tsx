import { useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { platformAuth } from '../lib/platform-api';

interface NavItem {
  path: string;
  label: string;
  icon: string;
}

const navItems: NavItem[] = [
  { path: '/platform/dashboard', label: 'Dashboard', icon: '▦' },
  { path: '/platform/tenants', label: 'Tenants', icon: '▤' },
  { path: '/platform/admins', label: 'Administrators', icon: '♙' },
  { path: '/platform/migrations', label: 'Database migrations', icon: '↻' },
];

export default function Sidebar() {
  const location = useLocation();
  const navigate = useNavigate();
  const [loggingOut, setLoggingOut] = useState(false);

  const logout = async () => {
    if (loggingOut) return;
    setLoggingOut(true);
    try {
      await platformAuth.logout();
    } catch {
      // Always clear local credentials even if the API is unavailable.
    } finally {
      localStorage.removeItem('platform_access_token');
      window.location.href = '/platform/login';
    }
  };

  return (
    <aside className="platform-sidebar">
      <div className="platform-brand">
        <div className="platform-brand-mark">G</div>
        <div>
          <strong>GoPass</strong>
          <span>Platform control</span>
        </div>
      </div>
      <div className="platform-nav-label">WORKSPACE</div>
      <nav className="platform-nav" aria-label="Platform administration">
        {navItems.map((item) => {
          const active = location.pathname === item.path ||
            (item.path !== '/platform/dashboard' && location.pathname.startsWith(item.path));
          return (
            <button
              key={item.path}
              type="button"
              className={`platform-nav-item${active ? ' active' : ''}`}
              aria-current={active ? 'page' : undefined}
              onClick={() => navigate(item.path)}
            >
              <span className="platform-nav-icon" aria-hidden="true">{item.icon}</span>
              <span>{item.label}</span>
              {active && <span className="platform-nav-active-dot" aria-hidden="true" />}
            </button>
          );
        })}
      </nav>
      <div className="platform-sidebar-bottom">
        <div className="platform-secure-label"><span /> Secured control plane</div>
        <button
          type="button"
          className="platform-logout"
          onClick={() => void logout()}
          disabled={loggingOut}
        >
          <span aria-hidden="true">↪</span> {loggingOut ? 'Signing out…' : 'Sign out'}
        </button>
      </div>
    </aside>
  );
}
