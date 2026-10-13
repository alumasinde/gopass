import api from './api';
import type { Envelope } from './api';

export interface PlatformAdmin {
  id: number;
  email: string;
  display_name: string;
  is_active: boolean;
  last_login_at?: string;
  created_at: string;
  updated_at: string;
}

export interface Organization {
  id: number;
  name: string;
  slug: string;
  is_active: boolean;
  lifecycle_status: 'ACTIVE' | 'SUSPENDED' | 'DEPROVISIONED' | 'PROVISIONING';
  database_mode: 'SHARED' | 'DEDICATED';
  provisioning_status: 'PENDING' | 'READY' | 'FAILED' | 'BLOCKED';
  provisioning_error?: string;
  provisioned_at?: string;
  suspended_at?: string;
  created_at: string;
  updated_at: string;
}

export interface LoginResponse {
  access_token: string;
  token_type: string;
  expires_at: string;
}

export interface CreateOrganizationInput {
  name: string;
  slug: string;
  database_mode?: 'SHARED' | 'DEDICATED';
  admin_first_name: string;
  admin_last_name: string;
  admin_email: string;
  admin_password: string;
}

export interface CreateAdminInput {
  email: string;
  display_name: string;
  password: string;
}

// Auth endpoints
export const platformAuth = {
  login: async (email: string, password: string): Promise<LoginResponse> => {
    const response = await api.post<Envelope<LoginResponse>>('/api/v1/platform/auth/login', {
      email,
      password,
    });
    return response.data.data;
  },

  logout: async (): Promise<void> => {
    await api.post('/api/v1/platform/auth/logout');
  },
};

// Admin management endpoints
export const platformAdmins = {
  list: async (): Promise<PlatformAdmin[]> => {
    const response = await api.get<Envelope<PlatformAdmin[]>>('/api/v1/platform/admins');
    return response.data.data;
  },

  create: async (data: CreateAdminInput): Promise<PlatformAdmin> => {
    const response = await api.post<Envelope<PlatformAdmin>>('/api/v1/platform/admins', data);
    return response.data.data;
  },

  suspend: async (id: number): Promise<void> => {
    await api.patch(`/api/v1/platform/admins/${id}/suspend`);
  },
};

// Organization management endpoints
export const platformOrganizations = {
  list: async (): Promise<Organization[]> => {
    const response = await api.get<Envelope<Organization[]>>('/api/v1/platform/organizations');
    return response.data.data;
  },

  get: async (id: number): Promise<Organization> => {
    const response = await api.get<Envelope<Organization>>(`/api/v1/platform/organizations/${id}`);
    return response.data.data;
  },

  create: async (data: CreateOrganizationInput): Promise<Organization> => {
    const response = await api.post<Envelope<Organization>>('/api/v1/platform/organizations', data);
    return response.data.data;
  },

  suspend: async (id: number): Promise<void> => {
    await api.patch(`/api/v1/platform/organizations/${id}/suspend`);
  },

  resume: async (id: number): Promise<void> => {
    await api.patch(`/api/v1/platform/organizations/${id}/resume`);
  },

  deprovision: async (id: number): Promise<void> => {
    await api.post(`/api/v1/platform/organizations/${id}/deprovision`);
  },

  retryProvisioning: async (id: number): Promise<void> => {
    await api.post(`/api/v1/platform/organizations/${id}/provision/retry`);
  },

  setDatabaseConfig: async (id: number, dsn: string): Promise<void> => {
    await api.post(`/api/v1/platform/organizations/${id}/database-config`, { dsn });
  },
};


export interface MigrationEntry {
  version: number;
  name: string;
  applied: boolean;
  applied_at?: string;
  checksum_status: 'NOT_APPLIED' | 'UNVERIFIED' | 'MATCH' | 'MISMATCH' | 'NAME_MISMATCH' | 'UNKNOWN_MIGRATION';
  drift: boolean;
}

export interface MigrationReport {
  history_exists: boolean;
  has_checksums: boolean;
  migrations: MigrationEntry[];
  pending_count: number;
  applied_count: number;
  drift_count: number;
}

export interface MigrationStatusResponse {
  target_database: string;
  execution_enabled: boolean;
  report: MigrationReport;
  active_run?: {
    run_id: string;
    status: 'QUEUED' | 'RUNNING';
    target_database: string;
    expected_versions: number[];
    updated_at: string;
  };
  latest_run?: MigrationRun;
}

export interface MigrationRun {
  run_id: string;
  requested_by: number;
  target_database: string;
  status: 'QUEUED' | 'RUNNING' | 'SUCCEEDED' | 'FAILED';
  expected_versions: number[];
  applied_versions: number[];
  error_message?: string;
  created_at: string;
  updated_at: string;
  started_at?: string;
  finished_at?: string;
}

export const platformMigrations = {
  status: async (): Promise<MigrationStatusResponse> => {
    const response = await api.get<Envelope<MigrationStatusResponse>>('/api/v1/platform/migrations');
    return response.data.data;
  },

  run: async (expectedVersions: number[], confirmation: string): Promise<{ run_id: string; status: string; expected_versions: number[]; message: string }> => {
    const response = await api.post<Envelope<{ run_id: string; status: string; expected_versions: number[]; message: string }>>(
      '/api/v1/platform/migrations/run',
      { expected_versions: expectedVersions, confirmation },
    );
    return response.data.data;
  },

  getRun: async (runID: string): Promise<MigrationRun> => {
    const response = await api.get<Envelope<MigrationRun>>(`/api/v1/platform/migrations/runs/${encodeURIComponent(runID)}`);
    return response.data.data;
  },
};
