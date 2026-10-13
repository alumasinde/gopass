import axios from 'axios';

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';

/** The backend wraps every success response as { data: ... }. */
export interface Envelope<T> {
  data: T;
}

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Add auth token to requests
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('platform_access_token');
  if (token && token !== 'undefined' && token !== 'null') {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Handle expired/invalid sessions (but not a failed login attempt)
api.interceptors.response.use(
  (response) => response,
  (error) => {
    const isLoginCall = String(error.config?.url ?? '').includes('/platform/auth/login');
    if (error.response?.status === 401 && !isLoginCall) {
      localStorage.removeItem('platform_access_token');
      if (window.location.pathname !== '/platform/login') {
        window.location.href = '/platform/login';
      }
    }
    return Promise.reject(error);
  }
);

/** The backend returns errors as { error: { code, message } }. */
export function apiErrorMessage(err: unknown, fallback: string): string {
  const e = err as { response?: { data?: { error?: { message?: string }; message?: string } } };
  return e?.response?.data?.error?.message || e?.response?.data?.message || fallback;
}

export default api;
