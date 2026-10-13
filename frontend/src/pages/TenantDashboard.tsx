import { useCallback, useEffect, useState } from 'react';
import { useTenantAuth } from '../contexts/TenantAuthContext';
import { tenantDashboardApi } from '../lib/tenant-api';
import type { ApprovalRequest, CheckIn, CheckOut, Gatepass, Visitor } from '../lib/tenant-api';
import { tenantApiErrorMessage } from '../lib/tenant-api';

type DashboardData = {
  visitors: Visitor[];
  gatepasses: Gatepass[];
  approvals: ApprovalRequest[];
  checkIns: CheckIn[];
  checkOuts: CheckOut[];
};

type LoadState = { loading: boolean; error: string; unavailable: string[] };
const emptyData: DashboardData = { visitors: [], gatepasses: [], approvals: [], checkIns: [], checkOuts: [] };

function formatDate(value?: string) {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat('en-KE', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Africa/Nairobi' }).format(date);
}
function titleCase(value: string) { return value.toLowerCase().replace(/_/g, ' ').replace(/\b\w/g, (letter) => letter.toUpperCase()); }

export default function TenantDashboard() {
  const { user, organization, reloadIdentity } = useTenantAuth();
  const [data, setData] = useState<DashboardData>(emptyData);
  const [state, setState] = useState<LoadState>({ loading: true, error: '', unavailable: [] });

  const load = useCallback(async () => {
    setState((current) => ({ ...current, loading: true, error: '' }));
    try {
      const results = await Promise.allSettled([
        tenantDashboardApi.visitors(), tenantDashboardApi.gatepasses(), tenantDashboardApi.approvals(),
        tenantDashboardApi.checkIns(), tenantDashboardApi.checkOuts(), reloadIdentity(),
      ]);
      const keys = ['visitors', 'gatepasses', 'approvals', 'check-ins', 'check-outs'] as const;
      const next: DashboardData = { ...emptyData };
      const unavailable: string[] = [];
      const setters: Array<(value: never) => void> = [
        ((value: Visitor[]) => { next.visitors = value; }) as (value: never) => void,
        ((value: Gatepass[]) => { next.gatepasses = value; }) as (value: never) => void,
        ((value: ApprovalRequest[]) => { next.approvals = value; }) as (value: never) => void,
        ((value: CheckIn[]) => { next.checkIns = value; }) as (value: never) => void,
        ((value: CheckOut[]) => { next.checkOuts = value; }) as (value: never) => void,
      ];
      results.slice(0, 5).forEach((result, index) => {
        if (result.status === 'fulfilled') setters[index](result.value as never);
        else unavailable.push(keys[index]);
      });
      const identity = results[5];
      if (identity.status === 'rejected' && unavailable.length === 5) {
        throw identity.reason;
      }
      setData(next);
      setState({ loading: false, error: '', unavailable });
    } catch (error) {
      setState({ loading: false, error: tenantApiErrorMessage(error, 'Unable to load your workspace. Please try again.'), unavailable: [] });
    }
  }, [reloadIdentity]);

  useEffect(() => { void load(); }, [load]);

  const pendingApprovals = data.approvals.filter((item) => item.status.toUpperCase() === 'PENDING').length;
  const pendingPasses = data.gatepasses.filter((item) => ['PENDING', 'PENDING_APPROVAL'].includes(item.status.toUpperCase())).length;
  const checkedOutIDs = new Set(data.checkOuts.map((item) => item.gatepass_id));
  const onSiteIDs = new Set(data.checkIns.filter((item) => !checkedOutIDs.has(item.gatepass_id)).map((item) => item.gatepass_id));
  const recentPasses = [...data.gatepasses].sort((a, b) => new Date(b.valid_from).getTime() - new Date(a.valid_from).getTime()).slice(0, 5);
  const visitorsByID = new Map(data.visitors.map((visitor) => [visitor.id, visitor]));

  return (
    <div className="tenant-dashboard-page">
      <section className="tenant-welcome-row"><div><span className="eyebrow">OVERVIEW</span><h1>Good to see you, {user?.first_name || 'there'}.</h1><p>Here is what is happening across {organization?.name || 'your organization'}.</p></div><button className="tenant-outline-button" onClick={() => void load()} disabled={state.loading}><span className={state.loading ? 'tenant-spin-icon' : ''}>↻</span> {state.loading ? 'Refreshing…' : 'Refresh data'}</button></section>

      {state.error && <div className="tenant-alert tenant-alert-error" role="alert"><strong>Dashboard could not load.</strong><span>{state.error}</span><button onClick={() => void load()}>Try again</button></div>}
      {state.unavailable.length > 0 && <div className="tenant-alert tenant-alert-info">Some totals are hidden because your account may not have permission to view: {state.unavailable.join(', ')}. Available totals below are from live API responses.</div>}

      <section className="tenant-stat-grid" aria-label="Workspace summary">
        <StatCard label="Visitors" value={state.unavailable.includes('visitors') ? null : data.visitors.length} description="Registered visitor records" icon="♙" tone="teal" loading={state.loading} />
        <StatCard label="Gatepasses" value={state.unavailable.includes('gatepasses') ? null : data.gatepasses.length} description={state.unavailable.includes('gatepasses') ? 'Not available for this account' : `${pendingPasses} awaiting approval or action`} icon="▤" tone="blue" loading={state.loading} />
        <StatCard label="Pending approvals" value={state.unavailable.includes('approvals') ? null : pendingApprovals} description="Requests waiting for a decision" icon="◷" tone="amber" loading={state.loading} />
        <StatCard label="Currently on site" value={state.unavailable.some((item) => item === 'check-ins' || item === 'check-outs') ? null : onSiteIDs.size} description="Based on check-in and check-out records" icon="⌖" tone="violet" loading={state.loading} />
      </section>

      <section className="tenant-dashboard-grid">
        <div className="tenant-panel tenant-recent-panel"><div className="tenant-panel-heading"><div><h2>Recent gatepasses</h2><p>Latest pass records returned by your workspace API.</p></div><span className="tenant-panel-count">{data.gatepasses.length} total</span></div>
          {state.loading ? <LoadingRows /> : state.unavailable.includes('gatepasses') ? <EmptyState title="Gatepass data unavailable" text="Your account may not have permission to view gatepasses." /> : recentPasses.length === 0 ? <EmptyState title="No gatepasses yet" text="Once gatepasses are created, the latest records will appear here." /> : <div className="tenant-table-wrap"><table className="tenant-table"><thead><tr><th>Pass</th><th>Visitor</th><th>Status</th><th>Valid from</th></tr></thead><tbody>{recentPasses.map((pass) => { const visitor = visitorsByID.get(pass.visitor_id); return <tr key={pass.id}><td><strong>GP-{String(pass.id).padStart(4, '0')}</strong><small>Gate #{pass.gate_id}</small></td><td>{visitor ? `${visitor.first_name} ${visitor.last_name}` : `Visitor #${pass.visitor_id}`}</td><td><span className={`tenant-status status-${pass.status.toLowerCase()}`}>{titleCase(pass.status)}</span></td><td>{formatDate(pass.valid_from)}</td></tr>; })}</tbody></table></div>}
        </div>
        <div className="tenant-panel tenant-activity-panel"><div className="tenant-panel-heading"><div><h2>Workspace activity</h2><p>Recent gate movement from live records.</p></div></div>
          {state.loading ? <LoadingRows /> : state.unavailable.includes('check-ins') && state.unavailable.includes('check-outs') ? <EmptyState title="Gate activity unavailable" text="Your account may not have permission to view check-in and check-out records." /> : <div className="tenant-activity-list">
            {data.checkIns.slice(-4).reverse().map((item) => <div className="tenant-activity-item" key={`in-${item.id}`}><span className="tenant-activity-icon activity-in">↘</span><div><strong>Check-in recorded</strong><span>Gatepass #{item.gatepass_id} · Gate #{item.gate_id}</span></div><time>{formatDate(item.checked_in_at)}</time></div>)}
            {data.checkOuts.slice(-3).reverse().map((item) => <div className="tenant-activity-item" key={`out-${item.id}`}><span className="tenant-activity-icon activity-out">↗</span><div><strong>Check-out recorded</strong><span>Gatepass #{item.gatepass_id} · Gate #{item.gate_id}</span></div><time>{formatDate(item.checked_out_at)}</time></div>)}
            {data.checkIns.length === 0 && data.checkOuts.length === 0 && <EmptyState title="No gate activity yet" text="Check-in and check-out activity will appear here when recorded." />}
          </div>}
        </div>
      </section>

      <section className="tenant-dashboard-footer"><div><span className="tenant-live-dot" /><div><strong>Connected to GoPass API</strong><span>Dashboard data is loaded from your authenticated tenant endpoints.</span></div></div><span>Times shown in East Africa Time (EAT)</span></section>
    </div>
  );
}

function StatCard({ label, value, description, icon, tone, loading }: { label: string; value: number | null; description: string; icon: string; tone: string; loading: boolean }) {
  return <article className="tenant-stat-card"><div className={`tenant-stat-icon tone-${tone}`}>{icon}</div><div className="tenant-stat-label">{label}</div><div className="tenant-stat-value">{loading ? <span className="tenant-skeleton-value" /> : value === null ? '—' : value.toLocaleString('en-KE')}</div><div className="tenant-stat-description">{description}</div></article>;
}
function LoadingRows() { return <div className="tenant-loading-rows"><span /><span /><span /></div>; }
function EmptyState({ title, text }: { title: string; text: string }) { return <div className="tenant-empty-state"><span>◌</span><strong>{title}</strong><p>{text}</p></div>; }
