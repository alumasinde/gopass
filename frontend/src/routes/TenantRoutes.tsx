import { Navigate } from 'react-router-dom';
import type { ReactNode } from 'react';
import { useTenantAuth } from '../contexts/TenantAuthContext';
import TenantLayout from '../components/TenantLayout';
import TenantLogin from '../pages/TenantLogin';
import TenantDashboard from '../pages/TenantDashboard';

function TenantProtectedRoute({ children }: { children: ReactNode }) {
  const { isAuthenticated, isLoading } = useTenantAuth();
  if (isLoading) return <div className="tenant-route-loading"><span className="tenant-spinner" /> Checking your session…</div>;
  if (!isAuthenticated) return <Navigate to="/tenant/login" replace />;
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
  { path: '/tenant', element: <Navigate to="/tenant/dashboard" replace /> },
];
