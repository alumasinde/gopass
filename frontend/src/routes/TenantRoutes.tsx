import { Link, Navigate } from 'react-router-dom';
import type { ReactNode } from 'react';
import { useTenantAuth } from '../contexts/TenantAuthContext';
import TenantLayout from '../components/TenantLayout';
import TenantLogin from '../modules/auth/TenantLogin';
import TenantDashboard from '../modules/dashboard/TenantDashboard';
import VisitorsPage from '../modules/visitors/VisitorsPage';
import VisitsPage from '../modules/visits/VisitsPage';

function TenantProtectedRoute({ children, permission }: { children: ReactNode; permission?: string }) {
  const { isAuthenticated, isLoading, user } = useTenantAuth();
  if (isLoading) return <div className="tenant-route-loading"><span className="tenant-spinner" /> Checking your session…</div>;
  if (!isAuthenticated) return <Navigate to="/tenant/login" replace />;
  if (permission && !(user?.permissions ?? []).includes(permission)) return <div className="module-page module-access-denied" role="alert"><span className="module-icon">⌁</span><h1>Permission required</h1><p>Your role does not have <code>{permission}</code>. Ask your organization administrator to update your role.</p><Link className="module-button module-button-secondary" to="/tenant/dashboard">Return to dashboard</Link></div>;
  return <>{children}</>;
}

function TenantPublicRoute({ children }: { children: ReactNode }) {
  const { isAuthenticated, isLoading } = useTenantAuth();
  if (isLoading) return <div className="tenant-route-loading"><span className="tenant-spinner" /> Checking your session…</div>;
  if (isAuthenticated) return <Navigate to="/tenant/dashboard" replace />;
  return <>{children}</>;
}

export const tenantRoutes = [
  { path: '/tenant/login', element: <TenantPublicRoute><TenantLogin /></TenantPublicRoute> },
  { path: '/tenant/dashboard', element: <TenantProtectedRoute><TenantLayout><TenantDashboard /></TenantLayout></TenantProtectedRoute> },
  { path: '/tenant/visitors', element: <TenantProtectedRoute permission="visitors.view"><TenantLayout><VisitorsPage /></TenantLayout></TenantProtectedRoute> },
  { path: '/tenant/visits', element: <TenantProtectedRoute permission="gatepasses.view"><TenantLayout><VisitsPage /></TenantLayout></TenantProtectedRoute> },
  { path: '/tenant', element: <Navigate to="/tenant/dashboard" replace /> },
];
