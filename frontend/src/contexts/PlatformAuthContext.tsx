import { createContext, useContext, useState, useEffect } from 'react';
import type { ReactNode } from 'react';
import { platformAuth } from '../lib/platform-api';

interface PlatformAuthContextType {
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
}

const PlatformAuthContext = createContext<PlatformAuthContextType | undefined>(undefined);

export const usePlatformAuth = () => {
  const context = useContext(PlatformAuthContext);
  if (!context) {
    throw new Error('usePlatformAuth must be used within a PlatformAuthProvider');
  }
  return context;
};

export const PlatformAuthProvider = ({ children }: { children: ReactNode }) => {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const token = localStorage.getItem('platform_access_token');
    setIsAuthenticated(!!token);
    setIsLoading(false);
  }, []);

  const login = async (email: string, password: string) => {
    const response = await platformAuth.login(email, password);
    localStorage.setItem('platform_access_token', response.access_token);
    setIsAuthenticated(true);
  };

  const logout = async () => {
    try {
      await platformAuth.logout();
    } catch (error) {
      console.error('Logout error:', error);
    } finally {
      localStorage.removeItem('platform_access_token');
      setIsAuthenticated(false);
    }
  };

  return (
    <PlatformAuthContext.Provider value={{ isAuthenticated, isLoading, login, logout }}>
      {children}
    </PlatformAuthContext.Provider>
  );
};
