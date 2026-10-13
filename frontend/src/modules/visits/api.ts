import tenantApi from '../../lib/tenant-api';
import type {Envelope,Visit,VisitInput,VisitUpdateInput,TenantSite,TenantGate,Visitor} from '../../lib/tenant-api';
const getData=async<T>(url:string)=>{const r=await tenantApi.get<Envelope<T>>(url);return r.data.data};
const mutate=async<T>(request:Promise<{data:Envelope<T>}>)=>(await request).data.data;
export const visitsApi={list:()=>getData<Visit[]>('/api/v1/visits/'),get:(id:number)=>getData<Visit>(`/api/v1/visits/${id}`),create:(input:VisitInput)=>mutate(tenantApi.post<Envelope<Visit>>('/api/v1/visits/',input)),update:(id:number,input:VisitUpdateInput)=>mutate(tenantApi.put<Envelope<{id:number}>>(`/api/v1/visits/${id}`,input)),delete:(id:number)=>mutate(tenantApi.delete<Envelope<{id:number}>>(`/api/v1/visits/${id}`)),sites:()=>getData<TenantSite[]>('/api/v1/sites/'),gates:()=>getData<TenantGate[]>('/api/v1/gates/'),visitors:()=>getData<Visitor[]>('/api/v1/visitors/')};
