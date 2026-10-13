import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { PlatformAuthProvider } from './contexts/PlatformAuthContext';
import { platformRoutes } from './routes/PlatformRoutes';
import { TenantAuthProvider } from './contexts/TenantAuthContext';
import { tenantRoutes } from './routes/TenantRoutes';

function App() {
  return (
    <BrowserRouter>
      <PlatformAuthProvider>
        <TenantAuthProvider>
          <Routes>
            {[...platformRoutes, ...tenantRoutes].map((route) => (
              <Route key={route.path} path={route.path} element={route.element} />
            ))}
          </Routes>
        </TenantAuthProvider>
      </PlatformAuthProvider>
    </BrowserRouter>
  );
}

export default App;
