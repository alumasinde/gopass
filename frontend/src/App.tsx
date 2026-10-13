import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { PlatformAuthProvider } from './contexts/PlatformAuthContext';
import { platformRoutes } from './routes/PlatformRoutes';

function App() {
  return (
    <BrowserRouter>
      <PlatformAuthProvider>
        <Routes>
          {platformRoutes.map((route) => (
            <Route key={route.path} path={route.path} element={route.element} />
          ))}
        </Routes>
      </PlatformAuthProvider>
    </BrowserRouter>
  );
}

export default App;
