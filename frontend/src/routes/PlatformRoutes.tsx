import { Navigate } from 'react-router-dom';
import { usePlatformAuth } from '../contexts/PlatformAuthContext';
import PlatformLogin from '../pages/PlatformLogin';
import PlatformDashboard from '../pages/PlatformDashboard';
import PlatformLayout from '../components/PlatformLayout';
import Tenants from '../pages/Tenants';
import TenantDetail from '../pages/TenantDetail';
import Admins from '../pages/Admins';
import Migrations from '../pages/Migrations';

export function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading } = usePlatformAuth();

  if (isLoading) {
    return <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh' }}>Loading...</div>;
  }

  if (!isAuthenticated) {
    return <Navigate to="/platform/login" replace />;
  }

  return <>{children}</>;
}

export function PublicRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading } = usePlatformAuth();

  if (isLoading) {
    return <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh' }}>Loading...</div>;
  }

  if (isAuthenticated) {
    return <Navigate to="/platform/dashboard" replace />;
  }

  return <>{children}</>;
}

export const platformRoutes = [
  {
    path: '/platform/login',
    element: (
      <PublicRoute>
        <PlatformLogin />
      </PublicRoute>
    ),
  },
  {
    path: '/platform/dashboard',
    element: (
      <ProtectedRoute>
        <PlatformLayout>
          <PlatformDashboard />
        </PlatformLayout>
      </ProtectedRoute>
    ),
  },
  {
    path: '/platform/tenants',
    element: (
      <ProtectedRoute>
        <PlatformLayout>
          <Tenants />
        </PlatformLayout>
      </ProtectedRoute>
    ),
  },
  {
    path: '/platform/tenants/:id',
    element: (
      <ProtectedRoute>
        <PlatformLayout>
          <TenantDetail />
        </PlatformLayout>
      </ProtectedRoute>
    ),
  },
  {
    path: '/platform/admins',
    element: (
      <ProtectedRoute>
        <PlatformLayout>
          <Admins />
        </PlatformLayout>
      </ProtectedRoute>
    ),
  },
  {
    path: '/platform/migrations',
    element: (
      <ProtectedRoute>
        <PlatformLayout>
          <Migrations />
        </PlatformLayout>
      </ProtectedRoute>
    ),
  },
  {
    path: '/',
    element: <Navigate to="/platform/dashboard" replace />,
  },
];
