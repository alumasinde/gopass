import tenantApi from '../../lib/tenant-api';
const data=async<T>(p:Promise<{data:{data:T}}>)=>(await p).data.data;
export const passTypesApi={list:()=>data(tenantApi.get('/api/v1/pass-types/')) as Promise<any[]>,create:(v:Record<string,unknown>)=>data(tenantApi.post('/api/v1/pass-types/',v)),update:(id:number,v:Record<string,unknown>)=>data(tenantApi.put(`/api/v1/pass-types/${id}`,v)),remove:(id:number)=>data(tenantApi.delete(`/api/v1/pass-types/${id}`))};
