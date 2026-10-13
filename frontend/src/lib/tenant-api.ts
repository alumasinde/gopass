import axios from 'axios';

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';

export interface Envelope<T> { data: T }
export interface TenantUser {
  id: number;
  organization_id?: number;
  first_name: string;
  last_name: string;
  email: string;
  is_active?: boolean;
  permissions?: string[];
  roles?: string[];
  organization_name?: string;
  organization_slug?: string;
}
export interface TenantOrganization {
  id: number;
  name: string;
  slug: string;
  is_active: boolean;
  branding?: { logo_url?: string; primary_color?: string };
}
export interface TenantLoginResponse {
  user: TenantUser;
  access_token: string;
  refresh_token: string;
}
export interface Visitor {
  id: number;
  first_name: string;
  last_name: string;
  phone?: string;
  email?: string;
  is_blacklisted?: boolean;
}
export interface Gatepass {
  id: number;
  organization_id?: number;
  visitor_id: number;
  gate_id: number;
  pass_type_id: number;
  status: string;
  valid_from: string;
  valid_until: string;
}
export interface TenantGate { id: number; site_id: number; name: string; code: string; is_active: boolean }
export interface PassType { id: number; name: string; code: string; requires_approval: boolean; validity_minutes: number; is_active: boolean }
export interface VisitorInput { first_name: string; last_name: string; phone: string; email: string }
export interface CreateGatepassInput { visitor_id: number; gate_id: number; pass_type_id: number; valid_from: string; valid_until: string }
export interface ApprovalRequest {
  id: number;
  gatepass_id: number;
  status: string;
  step_order: number;
}
export interface CheckIn {
  id: number;
  gatepass_id: number;
  gate_id: number;
  checked_in_at: string;
}
export interface CheckOut {
  id: number;
  gatepass_id: number;
  gate_id: number;
  checked_out_at: string;
}

const tenantApi = axios.create({
  baseURL: API_BASE_URL,
  headers: { 'Content-Type': 'application/json' },
});

tenantApi.interceptors.request.use((config) => {
  const token = localStorage.getItem('tenant_access_token');
  if (token && token !== 'undefined' && token !== 'null') {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

tenantApi.interceptors.response.use(
  (response) => response,
  (error) => {
    const url = String(error.config?.url ?? '');
    const isAuthRequest = url.includes('/auth/login') || url.includes('/auth/refresh') || url.includes('/auth/logout');
    if (error.response?.status === 401 && !isAuthRequest) {
      localStorage.removeItem('tenant_access_token');
      localStorage.removeItem('tenant_refresh_token');
      localStorage.removeItem('tenant_user');
      if (window.location.pathname.startsWith('/tenant') && window.location.pathname !== '/tenant/login') {
        window.location.href = '/tenant/login';
      }
    }
    return Promise.reject(error);
  },
);

export function tenantApiErrorMessage(err: unknown, fallback: string): string {
  const error = err as { response?: { data?: { error?: { message?: string }; message?: string } }; message?: string };
  return error?.response?.data?.error?.message || error?.response?.data?.message || error?.message || fallback;
}

async function getData<T>(url: string): Promise<T> {
  const response = await tenantApi.get<Envelope<T>>(url);
  return response.data.data;
}

async function getArrayData<T>(url: string): Promise<T[]> {
  const value = await getData<unknown>(url);
  if (Array.isArray(value)) return value as T[];
  if (value && typeof value === 'object') {
    const record = value as { data?: unknown; items?: unknown };
    if (Array.isArray(record.data)) return record.data as T[];
    if (Array.isArray(record.items)) return record.items as T[];
  }
  return [];
}

export const tenantAuthApi = {
  login: async (organizationSlug: string, email: string, password: string) => {
    const response = await tenantApi.post<Envelope<TenantLoginResponse>>('/api/v1/auth/login', {
      organization_slug: organizationSlug.trim().toLowerCase(),
      tenant_host: window.location.hostname.toLowerCase(),
      email: email.trim().toLowerCase(),
      password,
    });
    return response.data.data;
  },
  me: () => getData<TenantUser>('/api/v1/auth/me'),
  organization: () => getData<TenantOrganization>('/api/v1/organization'),
  resolveHost: async (host: string) => {
    const response = await tenantApi.get<Envelope<{ slug: string; name: string }>>('/api/v1/tenant/resolve', { params: { host } });
    return response.data.data;
  },
  logout: async () => {
    const refreshToken = localStorage.getItem('tenant_refresh_token');
    if (refreshToken) await tenantApi.post('/api/v1/auth/logout', { refresh_token: refreshToken });
  },
};

export const tenantDashboardApi = {
  visitors: () => getArrayData<Visitor>('/api/v1/visitors/'),
  gatepasses: () => getArrayData<Gatepass>('/api/v1/gatepasses/'),
  approvals: () => getArrayData<ApprovalRequest>('/api/v1/approvals/'),
  checkIns: () => getArrayData<CheckIn>('/api/v1/check-ins/'),
  checkOuts: () => getArrayData<CheckOut>('/api/v1/check-outs/'),
};

export const tenantVisitorsApi = {
  list: () => getArrayData<Visitor>('/api/v1/visitors/'),
  get: (id: number) => getData<Visitor>(`/api/v1/visitors/${id}`),
  create: async (input: VisitorInput) => { const response = await tenantApi.post<Envelope<Visitor>>('/api/v1/visitors/', input); return response.data.data; },
  update: async (id: number, input: VisitorInput) => { const response = await tenantApi.put<Envelope<{ id: number }>>(`/api/v1/visitors/${id}`, input); return response.data.data; },
  blacklist: async (id: number) => { const response = await tenantApi.post<Envelope<{ id: number; is_blacklisted: boolean }>>(`/api/v1/visitors/${id}/blacklist`); return response.data.data; },
  unblacklist: async (id: number) => { const response = await tenantApi.post<Envelope<{ id: number; is_blacklisted: boolean }>>(`/api/v1/visitors/${id}/unblacklist`); return response.data.data; },
};

export const tenantVisitsApi = {
  list: () => getArrayData<Gatepass>('/api/v1/gatepasses/'),
  gates: () => getArrayData<TenantGate>('/api/v1/gates/'),
  passTypes: () => getArrayData<PassType>('/api/v1/pass-types/'),
  create: async (input: CreateGatepassInput) => { const response = await tenantApi.post<Envelope<Gatepass>>('/api/v1/gatepasses/', input); return response.data.data; },
  submit: async (id: number) => { const response = await tenantApi.post<Envelope<{ id: number; status: string }>>(`/api/v1/gatepasses/${id}/submit`); return response.data.data; },
  cancel: async (id: number) => { const response = await tenantApi.post<Envelope<{ id: number; status: string }>>(`/api/v1/gatepasses/${id}/cancel`); return response.data.data; },
  revoke: async (id: number) => { const response = await tenantApi.post<Envelope<{ id: number; status: string }>>(`/api/v1/gatepasses/${id}/revoke`); return response.data.data; },
};

export default tenantApi;
