import { Navigate } from 'react-router-dom';
import { usePlatformAuth } from '../contexts/PlatformAuthContext';
import PlatformLogin from '../pages/PlatformLogin';
import PlatformDashboard from '../pages/PlatformDashboard';

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
        <PlatformDashboard />
      </ProtectedRoute>
    ),
  },
  {
    path: '/',
    element: <Navigate to="/platform/dashboard" replace />,
  },
];
