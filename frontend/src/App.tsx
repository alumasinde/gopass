import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { PlatformAuthProvider } from './contexts/PlatformAuthContext';
import { platformRoutes } from './routes/PlatformRoutes';
import { TenantAuthProvider } from './contexts/TenantAuthContext';
import { tenantRoutes } from './routes/TenantRoutes';
import AppErrorBoundary from './components/AppErrorBoundary';
import { isTenantHostname } from './lib/tenant-host';

function RootRedirect() {
  return <Navigate to={isTenantHostname(window.location.hostname) ? '/tenant/login' : '/platform/dashboard'} replace />;
}

function App() {
  return (
    <AppErrorBoundary>
      <BrowserRouter>
        <PlatformAuthProvider>
          <TenantAuthProvider>
            <Routes>
              {[...platformRoutes.filter((route) => route.path !== '/'), ...tenantRoutes].map((route) => (
                <Route key={route.path} path={route.path} element={route.element} />
              ))}
              <Route path="/" element={<RootRedirect />} />
              <Route path="*" element={<RootRedirect />} />
            </Routes>
          </TenantAuthProvider>
        </PlatformAuthProvider>
      </BrowserRouter>
    </AppErrorBoundary>
  );
}

export default App;
