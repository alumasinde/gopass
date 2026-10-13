import tenantApi from '../../lib/tenant-api';
import type { Envelope, IdentificationType, IdentificationTypeInput } from '../../lib/tenant-api';

const data = async <T,>(url: string): Promise<T> => (await tenantApi.get<Envelope<T>>(url)).data.data;
const mutate = async <T,>(request: Promise<{ data: Envelope<T> }>): Promise<T> => (await request).data.data;

export const identificationTypesApi = {
  list: () => data<IdentificationType[]>('/api/v1/identification-types/'),
  create: (input: IdentificationTypeInput) => mutate(tenantApi.post<Envelope<IdentificationType>>('/api/v1/identification-types/', input)),
  update: (id: number, input: Omit<IdentificationTypeInput, 'code'>) => mutate(tenantApi.put<Envelope<IdentificationType>>(`/api/v1/identification-types/${id}`, input)),
  delete: (id: number) => mutate(tenantApi.delete<Envelope<{ message: string }>>(`/api/v1/identification-types/${id}`)),
};
