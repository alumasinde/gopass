import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { tenantAuthApi } from '../lib/tenant-api';
import type { TenantOrganization, TenantUser } from '../lib/tenant-api';

interface TenantAuthContextValue {
  user: TenantUser | null;
  organization: TenantOrganization | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (organizationSlug: string, email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  reloadIdentity: () => Promise<void>;
}

const TenantAuthContext = createContext<TenantAuthContextValue | undefined>(undefined);

export function useTenantAuth() {
  const context = useContext(TenantAuthContext);
  if (!context) throw new Error('useTenantAuth must be used within a TenantAuthProvider');
  return context;
}

export function TenantAuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<TenantUser | null>(null);
  const [organization, setOrganization] = useState<TenantOrganization | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  const clearSession = useCallback(() => {
    localStorage.removeItem('tenant_access_token');
    localStorage.removeItem('tenant_refresh_token');
    localStorage.removeItem('tenant_user');
    localStorage.removeItem('tenant_organization_slug');
    setUser(null);
    setOrganization(null);
  }, []);

  const reloadIdentity = useCallback(async () => {
    // The authenticated identity is required; organization.view is optional for
    // tenant users with narrower roles, so a forbidden organization profile must
    // not prevent them from signing in.
    const me = await tenantAuthApi.me();
    setUser(me);
    localStorage.setItem('tenant_user', JSON.stringify(me));
    try {
      const org = await tenantAuthApi.organization();
      setOrganization(org);
      localStorage.setItem('tenant_organization_slug', org.slug);
    } catch {
      const slug = localStorage.getItem('tenant_organization_slug') || '';
      setOrganization((current) => current ?? {
        id: me.organization_id ?? 0,
        name: me.organization_name || slug || 'Tenant workspace',
        slug: me.organization_slug || slug,
        is_active: me.is_active !== false,
      });
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    const token = localStorage.getItem('tenant_access_token');
    if (!token || token === 'undefined' || token === 'null') {
      clearSession();
      setIsLoading(false);
      return () => { cancelled = true; };
    }
    tenantAuthApi.me()
      .then(async (me) => {
        if (cancelled) return;
        setUser(me);
        localStorage.setItem('tenant_user', JSON.stringify(me));
        try {
          const org = await tenantAuthApi.organization();
          if (cancelled) return;
          setOrganization(org);
          localStorage.setItem('tenant_organization_slug', org.slug);
        } catch {
          if (cancelled) return;
          const slug = localStorage.getItem('tenant_organization_slug') || '';
          setOrganization({ id: me.organization_id ?? 0, name: me.organization_name || slug || 'Tenant workspace', slug: me.organization_slug || slug, is_active: me.is_active !== false });
        }
      })
      .catch(() => {
        if (!cancelled) clearSession();
      })
      .finally(() => { if (!cancelled) setIsLoading(false); });
    return () => { cancelled = true; };
    // Identity is intentionally verified once on mount against the backend.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [clearSession]);

  const login = useCallback(async (organizationSlug: string, email: string, password: string) => {
    const result = await tenantAuthApi.login(organizationSlug, email, password);
    if (!result.access_token || !result.refresh_token) throw new Error('Login response did not include session tokens');
    localStorage.setItem('tenant_access_token', result.access_token);
    localStorage.setItem('tenant_refresh_token', result.refresh_token);
    localStorage.setItem('tenant_organization_slug', organizationSlug.trim().toLowerCase());
    try {
      await reloadIdentity();
      setUser((current) => current ?? result.user);
    } catch (error) {
      clearSession();
      throw error;
    }
  }, [reloadIdentity, clearSession]);

  const logout = useCallback(async () => {
    try { await tenantAuthApi.logout(); } catch { /* Local logout must still complete. */ }
    clearSession();
  }, [clearSession]);

  const value = useMemo(() => ({
    user, organization, isAuthenticated: Boolean(user && localStorage.getItem('tenant_access_token')),
    isLoading, login, logout, reloadIdentity,
  }), [user, organization, isLoading, login, logout, reloadIdentity]);

  return <TenantAuthContext.Provider value={value}>{children}</TenantAuthContext.Provider>;
}
