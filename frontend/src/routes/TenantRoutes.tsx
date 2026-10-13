import { Link, Navigate } from 'react-router-dom';
import type { ReactNode } from 'react';
import { useTenantAuth } from '../contexts/TenantAuthContext';
import TenantLayout from '../components/TenantLayout';
import TenantLogin from '../modules/auth/TenantLogin';
import TenantDashboard from '../modules/dashboard/TenantDashboard';
import VisitorsPage from '../modules/visitors/VisitorsPage';
import VisitsPage from '../modules/visits/VisitsPage';
import SitesPage from '../modules/sites/SitesPage';
import GatesPage from '../modules/gates/GatesPage';
import PassTypesPage from '../modules/pass-types/PassTypesPage';
import UsersPage from '../modules/users/UsersPage';
import RolesPage from '../modules/roles/RolesPage';
import OrganizationSettingsPage from '../modules/organization/OrganizationSettingsPage';
import IdentificationTypesPage from '../modules/identification-types/IdentificationTypesPage';

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
  { path: '/tenant/sites', element: <TenantProtectedRoute permission="sites.view"><TenantLayout><SitesPage /></TenantLayout></TenantProtectedRoute> },
  { path: '/tenant/gates', element: <TenantProtectedRoute permission="gates.view"><TenantLayout><GatesPage /></TenantLayout></TenantProtectedRoute> },
  { path: '/tenant/identification-types', element: <TenantProtectedRoute permission="identification_types.view"><TenantLayout><IdentificationTypesPage /></TenantLayout></TenantProtectedRoute> },
  { path: '/tenant/pass-types', element: <TenantProtectedRoute permission="pass_types.view"><TenantLayout><PassTypesPage /></TenantLayout></TenantProtectedRoute> },
  { path: '/tenant/users', element: <TenantProtectedRoute permission="users.view"><TenantLayout><UsersPage /></TenantLayout></TenantProtectedRoute> },
  { path: '/tenant/roles', element: <TenantProtectedRoute permission="roles.view"><TenantLayout><RolesPage /></TenantLayout></TenantProtectedRoute> },
  { path: '/tenant/organization', element: <TenantProtectedRoute permission="organization.view"><TenantLayout><OrganizationSettingsPage /></TenantLayout></TenantProtectedRoute> },
  { path: '/tenant', element: <Navigate to="/tenant/dashboard" replace /> },
];
