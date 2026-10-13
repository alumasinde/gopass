import tenantApi from '../../lib/tenant-api';
export const usersApi={list:()=>tenantApi.get('/api/v1/users/'),create:(data:Record<string,unknown>)=>tenantApi.post('/api/v1/users/',data),update:(id:number,data:Record<string,unknown>)=>tenantApi.put(`/api/v1/users/${id}`,data),setActive:(id:number,is_active:boolean)=>tenantApi.patch(`/api/v1/users/${id}/status`,{is_active})};
