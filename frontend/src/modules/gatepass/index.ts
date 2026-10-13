/** Gatepass domain API. Gatepass records are also the current backend representation for scheduled visits. */
export { tenantVisitsApi as gatepassApi } from '../../lib/tenant-api';
export type { Gatepass, CreateGatepassInput, TenantGate, PassType } from '../../lib/tenant-api';
