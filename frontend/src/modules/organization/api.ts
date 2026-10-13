import tenantApi from '../../lib/tenant-api';
export const organizationApi={get:()=>tenantApi.get('/api/v1/organization'),update:(data:{name:string;slug:string})=>tenantApi.patch('/api/v1/organization',data)};
