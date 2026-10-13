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
}
export interface TenantOrganization {
  id: number;
  name: string;
  slug: string;
  is_active: boolean;
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
  visitor_id: number;
  gate_id: number;
  pass_type_id: number;
  status: string;
  valid_from: string;
  valid_until: string;
}
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

export const tenantAuthApi = {
  login: async (organizationSlug: string, email: string, password: string) => {
    const response = await tenantApi.post<Envelope<TenantLoginResponse>>('/api/v1/auth/login', {
      organization_slug: organizationSlug.trim().toLowerCase(),
      email: email.trim().toLowerCase(),
      password,
    });
    return response.data.data;
  },
  me: () => getData<TenantUser>('/api/v1/auth/me'),
  organization: () => getData<TenantOrganization>('/api/v1/organization'),
  logout: async () => {
    const refreshToken = localStorage.getItem('tenant_refresh_token');
    if (refreshToken) await tenantApi.post('/api/v1/auth/logout', { refresh_token: refreshToken });
  },
};

export const tenantDashboardApi = {
  visitors: () => getData<Visitor[]>('/api/v1/visitors/'),
  gatepasses: () => getData<Gatepass[]>('/api/v1/gatepasses/'),
  approvals: () => getData<ApprovalRequest[]>('/api/v1/approvals/'),
  checkIns: () => getData<CheckIn[]>('/api/v1/check-ins/'),
  checkOuts: () => getData<CheckOut[]>('/api/v1/check-outs/'),
};

export default tenantApi;
