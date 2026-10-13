import { useCallback, useEffect, useMemo, useState } from 'react';
import type { FormEvent } from 'react';
import { useTenantAuth } from '../../contexts/TenantAuthContext';
import { tenantApiErrorMessage, tenantDashboardApi } from '../../lib/tenant-api';
import { visitsApi } from './api';
import type { Gatepass, PassType, TenantGate, Visitor } from '../../lib/tenant-api';

function localDateTime(date: Date) {
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}
function formatDate(value?: string) {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('en-KE', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Africa/Nairobi' }).format(date);
}
function titleCase(value: string) { return value.toLowerCase().replace(/_/g, ' ').replace(/\b\w/g, (letter) => letter.toUpperCase()); }
function statusClass(value: string) {
  if (['APPROVED', 'ISSUED', 'CHECKED_IN'].includes(value)) return 'module-status-success';
  if (['PENDING_APPROVAL', 'DRAFT'].includes(value)) return 'module-status-warning';
  if (['REJECTED', 'CANCELLED', 'REVOKED'].includes(value)) return 'module-status-danger';
  return 'module-status-neutral';
}

type VisitForm = { visitor_id: string; gate_id: string; pass_type_id: string; valid_from: string; valid_until: string };
const defaultForm = (): VisitForm => {
  const from = new Date(Date.now() + 5 * 60_000);
  const until = new Date(from.getTime() + 60 * 60_000);
  return { visitor_id: '', gate_id: '', pass_type_id: '', valid_from: localDateTime(from), valid_until: localDateTime(until) };
};

export default function VisitsPage() {
  const { user } = useTenantAuth();
  const permissions = new Set(user?.permissions ?? []);
  const canView = permissions.has('gatepasses.view');
  const canViewVisitors = permissions.has('visitors.view');
  const canViewGates = permissions.has('gates.view');
  const canViewPassTypes = permissions.has('pass_types.view');
  const canCreate = permissions.has('gatepasses.create') && permissions.has('visitors.view') && permissions.has('gates.view') && permissions.has('pass_types.view');
  const canSubmit = permissions.has('gatepasses.submit');
  const canEdit = permissions.has('gatepasses.update');
  const canCancel = permissions.has('gatepasses.cancel');
  const canRevoke = permissions.has('gatepasses.revoke');
  const [passes, setPasses] = useState<Gatepass[]>([]);
  const [visitors, setVisitors] = useState<Visitor[]>([]);
  const [gates, setGates] = useState<TenantGate[]>([]);
  const [passTypes, setPassTypes] = useState<PassType[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [query, setQuery] = useState('');
  const [filter, setFilter] = useState('all');
  const [modalOpen, setModalOpen] = useState(false);
  const [editingPass, setEditingPass] = useState<Gatepass | null>(null);
  const [form, setForm] = useState<VisitForm>(defaultForm());
  const [formError, setFormError] = useState('');
  const [saving, setSaving] = useState(false);
  const [actionId, setActionId] = useState<number | null>(null);

  const load = useCallback(async (quiet = false) => {
    if (!canView) { setLoading(false); return; }
    if (quiet) setRefreshing(true); else setLoading(true);
    setError('');
    const results = await Promise.allSettled([
      visitsApi.list(),
      canViewVisitors ? tenantDashboardApi.visitors() : Promise.resolve([]),
      canViewGates ? visitsApi.gates() : Promise.resolve([]),
      canViewPassTypes ? visitsApi.passTypes() : Promise.resolve([]),
    ]);
    if (results[0].status === 'fulfilled') setPasses(results[0].value);
    else setError(tenantApiErrorMessage(results[0].reason, 'Visit records could not be loaded.'));
    if (results[1].status === 'fulfilled') setVisitors(results[1].value as Visitor[]);
    if (results[2].status === 'fulfilled') setGates(results[2].value as TenantGate[]);
    if (results[3].status === 'fulfilled') setPassTypes(results[3].value as PassType[]);
    const optionErrors = results.slice(1).filter((result) => result.status === 'rejected');
    if (optionErrors.length && results[0].status === 'fulfilled') setError('Visit records loaded, but some lookup data is unavailable. Check your visitors.view, gates.view and pass_types.view permissions.');
    setLoading(false); setRefreshing(false);
  }, [canView, canViewVisitors, canViewGates, canViewPassTypes]);

  useEffect(() => { void load(); }, [load]);

  const visitorById = useMemo(() => new Map(visitors.map((visitor) => [visitor.id, visitor])), [visitors]);
  const gateById = useMemo(() => new Map(gates.map((gate) => [gate.id, gate])), [gates]);
  const passTypeById = useMemo(() => new Map(passTypes.map((passType) => [passType.id, passType])), [passTypes]);
  const filtered = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return passes.filter((pass) => {
      if (filter !== 'all' && pass.status !== filter) return false;
      const visitor = visitorById.get(pass.visitor_id);
      const gate = gateById.get(pass.gate_id);
      const passType = passTypeById.get(pass.pass_type_id);
      return !normalized || [String(pass.id), visitor ? `${visitor.first_name} ${visitor.last_name}` : '', visitor?.email, visitor?.phone, gate?.name, passType?.name, pass.status]
        .some((value) => String(value ?? '').toLowerCase().includes(normalized));
    }).sort((a, b) => new Date(b.valid_from).getTime() - new Date(a.valid_from).getTime());
  }, [passes, filter, query, visitorById, gateById, passTypeById]);

  const submitCreate = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault(); setFormError('');
    const from = new Date(form.valid_from); const until = new Date(form.valid_until);
    if (!form.visitor_id || !form.gate_id || !form.pass_type_id) { setFormError('Select a visitor, gate and pass type.'); return; }
    if (Number.isNaN(from.getTime()) || Number.isNaN(until.getTime()) || until <= from) { setFormError('The end time must be later than the start time.'); return; }
    if (until.getTime() <= Date.now()) { setFormError('The visit end time must be in the future.'); return; }
    setSaving(true); setNotice('');
    try {
      const payload = { visitor_id: Number(form.visitor_id), gate_id: Number(form.gate_id), pass_type_id: Number(form.pass_type_id), valid_from: from.toISOString(), valid_until: until.toISOString() };
      const saved = editingPass ? await visitsApi.update(editingPass.id, payload) : await visitsApi.create(payload);
      setNotice(editingPass ? `Visit access request #${saved.id} updated.` : `Visit access request #${saved.id} created as a draft. Submit it when ready.`);
      setModalOpen(false); setEditingPass(null); setForm(defaultForm());
      await load(true);
    } catch (err) { setFormError(tenantApiErrorMessage(err, 'Visit could not be created. Check your gate and pass type permissions.')); }
    finally { setSaving(false); }
  };

  const openEdit = (pass: Gatepass) => {
    setEditingPass(pass);
    setForm({ visitor_id: String(pass.visitor_id), gate_id: String(pass.gate_id), pass_type_id: String(pass.pass_type_id), valid_from: localDateTime(new Date(pass.valid_from)), valid_until: localDateTime(new Date(pass.valid_until)) });
    setFormError(''); setModalOpen(true);
  };

  const runAction = async (pass: Gatepass, action: 'submit' | 'cancel' | 'revoke') => {
    const labels = { submit: 'submit this visit for processing', cancel: 'cancel this visit', revoke: 'revoke this visit access' };
    if (action !== 'submit' && !window.confirm(`Are you sure you want to ${labels[action]}?`)) return;
    setActionId(pass.id); setError(''); setNotice('');
    try {
      const result = action === 'submit' ? await visitsApi.submit(pass.id) : action === 'cancel' ? await visitsApi.cancel(pass.id) : await visitsApi.revoke(pass.id);
      setNotice(action === 'submit' ? `Visit #${pass.id} submitted. Current status: ${titleCase(result.status)}.` : `Visit #${pass.id} ${action === 'cancel' ? 'cancelled' : 'revoked'}.`);
      await load(true);
    } catch (err) { setError(tenantApiErrorMessage(err, `Could not ${action} visit #${pass.id}.`)); }
    finally { setActionId(null); }
  };

  if (!canView) return <section className="module-page"><div className="module-access-denied"><span className="module-icon">⌁</span><h1>Visits access required</h1><p>Your account does not have the <code>gatepasses.view</code> permission required to view scheduled access.</p></div></section>;

  const upcoming = passes.filter((pass) => new Date(pass.valid_until).getTime() >= Date.now() && !['CANCELLED', 'REVOKED', 'REJECTED', 'CHECKED_OUT'].includes(pass.status)).length;
  const pending = passes.filter((pass) => ['DRAFT', 'PENDING_APPROVAL'].includes(pass.status)).length;
  const active = passes.filter((pass) => ['APPROVED', 'ISSUED', 'CHECKED_IN'].includes(pass.status)).length;

  return <section className="module-page">
    <header className="module-page-header"><div><div className="module-eyebrow">WORKSPACE / ACCESS SCHEDULING</div><h1>Visits</h1><p>Schedule visitor access, track approval status and manage each visit's access lifecycle.</p></div><div className="module-header-actions"><button type="button" className="module-button module-button-secondary" onClick={() => void load(true)} disabled={refreshing}>{refreshing ? 'Refreshing…' : '↻ Refresh'}</button>{canCreate && <button type="button" className="module-button module-button-primary" onClick={() => { setEditingPass(null); setForm(defaultForm()); setFormError(''); setModalOpen(true); }}>＋ Schedule visit</button>}</div></header>
    <div className="module-info-banner"><span>i</span><p>Visit schedules currently use the backend's <strong>gatepass</strong> records. Creating a visit creates a draft gatepass; submit it to run the configured approval workflow.</p></div>
    <div className="module-metric-row"><article className="module-metric-card"><span className="module-metric-icon module-tone-blue">◷</span><div><small>Upcoming visits</small><strong>{upcoming}</strong></div></article><article className="module-metric-card"><span className="module-metric-icon module-tone-amber">⌛</span><div><small>Awaiting action</small><strong>{pending}</strong></div></article><article className="module-metric-card"><span className="module-metric-icon module-tone-green">✓</span><div><small>Approved / active</small><strong>{active}</strong></div></article></div>
    {notice && <div className="module-alert module-alert-success" role="status"><span>✓</span>{notice}<button type="button" aria-label="Dismiss notice" onClick={() => setNotice('')}>×</button></div>}
    {error && <div className="module-alert module-alert-error" role="alert"><span>!</span><div>{error}</div><button type="button" onClick={() => void load()} className="module-alert-action">Retry</button></div>}
    <div className="module-panel"><div className="module-toolbar"><div><h2>Visit schedule</h2><p>{filtered.length} visit records</p></div><div className="module-toolbar-controls"><label className="module-search"><span>⌕</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search visitor, gate, status…" aria-label="Search visits" /></label><select className="module-select" value={filter} onChange={(event) => setFilter(event.target.value)} aria-label="Filter visit status"><option value="all">All statuses</option>{['DRAFT', 'PENDING_APPROVAL', 'APPROVED', 'ISSUED', 'CHECKED_IN', 'CHECKED_OUT', 'REJECTED', 'CANCELLED', 'REVOKED'].map((status) => <option key={status} value={status}>{titleCase(status)}</option>)}</select></div></div>
      {loading ? <div className="module-table-loading"><span /><span /><span /><span /><span /></div> : filtered.length === 0 ? <div className="module-empty-state"><span className="module-empty-icon">◷</span><h3>{passes.length ? 'No matching visits' : 'No visits scheduled yet'}</h3><p>{passes.length ? 'Change the search or status filter.' : 'Schedule a visit to create a draft access request.'}</p>{canCreate && !passes.length && <button type="button" className="module-button module-button-primary" onClick={() => { setForm(defaultForm()); setModalOpen(true); }}>Schedule first visit</button>}</div> : <div className="module-table-scroll"><table className="module-table"><thead><tr><th>Visit</th><th>Visitor</th><th>Gate / pass type</th><th>Schedule</th><th>Status</th><th className="module-align-right">Actions</th></tr></thead><tbody>{filtered.map((pass) => { const visitor = visitorById.get(pass.visitor_id); const gate = gateById.get(pass.gate_id); const passType = passTypeById.get(pass.pass_type_id); const canSubmitThis = canSubmit && pass.status === 'DRAFT'; const canCancelThis = canCancel && ['DRAFT', 'PENDING_APPROVAL', 'APPROVED'].includes(pass.status); const canRevokeThis = canRevoke && ['APPROVED', 'ISSUED'].includes(pass.status); return <tr key={pass.id}>
        <td><strong className="module-primary-text">VISIT-{String(pass.id).padStart(5, '0')}</strong><small className="module-table-subtext">Access request #{pass.id}</small></td>
        <td><div className="module-inline-person"><span className="module-person-avatar">{visitor ? `${visitor.first_name?.[0] ?? ''}${visitor.last_name?.[0] ?? ''}`.toUpperCase() : 'V'}</span><span><strong>{visitor ? `${visitor.first_name} ${visitor.last_name}`.trim() : `Visitor #${pass.visitor_id}`}</strong><small>{visitor?.phone || visitor?.email || 'Contact not available'}</small></span></div></td>
        <td><strong>{gate?.name || `Gate #${pass.gate_id}`}</strong><small className="module-table-subtext">{passType?.name || `Pass type #${pass.pass_type_id}`}</small></td>
        <td><strong>{formatDate(pass.valid_from)}</strong><small className="module-table-subtext">Until {formatDate(pass.valid_until)}</small></td>
        <td><span className={`module-status ${statusClass(pass.status)}`}><i />{titleCase(pass.status)}</span></td>
        <td><div className="module-row-actions">{canEdit && pass.status === 'DRAFT' && <button type="button" className="module-button module-button-small module-button-secondary" disabled={actionId === pass.id} onClick={() => openEdit(pass)}>Edit</button>}{canSubmitThis && <button type="button" className="module-button module-button-small module-button-primary" disabled={actionId === pass.id} onClick={() => void runAction(pass, 'submit')}>{actionId === pass.id ? 'Working…' : 'Submit'}</button>}{canCancelThis && <button type="button" className="module-icon-button module-icon-button-danger" title="Cancel visit" aria-label="Cancel visit" disabled={actionId === pass.id} onClick={() => void runAction(pass, 'cancel')}>×</button>}{canRevokeThis && <button type="button" className="module-icon-button module-icon-button-danger" title="Revoke access" aria-label="Revoke access" disabled={actionId === pass.id} onClick={() => void runAction(pass, 'revoke')}>⊘</button>}</div></td>
      </tr>; })}</tbody></table></div>}
    </div>

    {modalOpen && <div className="module-modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget && !saving) { setModalOpen(false); setEditingPass(null); } }}><section className="module-modal module-modal-wide" role="dialog" aria-modal="true" aria-labelledby="visit-form-title"><header className="module-modal-header"><div><div className="module-eyebrow">VISIT SCHEDULING</div><h2 id="visit-form-title">{editingPass ? 'Edit visit draft' : 'Schedule a visit'}</h2><p>{editingPass ? 'Only draft access requests can be edited.' : 'Create a draft access request. You can submit it after saving.'}</p></div><button type="button" className="module-icon-button" aria-label="Close" disabled={saving} onClick={() => { setModalOpen(false); setEditingPass(null); }}>×</button></header>
      {(!visitors.length || !gates.some((gate) => gate.is_active) || !passTypes.some((type) => type.is_active)) && <div className="module-alert module-alert-warning"><span>!</span><div>Scheduling requires an available visitor, active gate and active pass type. Check the related modules and your permissions if any option is missing.</div></div>}
      <form onSubmit={(event) => void submitCreate(event)}><div className="module-form-grid"><label className="module-field module-field-full"><span>Visitor <b>*</b></span><select required value={form.visitor_id} onChange={(event) => setForm({ ...form, visitor_id: event.target.value })}><option value="">Select visitor</option>{visitors.filter((visitor) => !visitor.is_blacklisted).map((visitor) => <option key={visitor.id} value={visitor.id}>{visitor.first_name} {visitor.last_name}{visitor.phone ? ` · ${visitor.phone}` : ''}</option>)}</select></label><label className="module-field"><span>Gate <b>*</b></span><select required value={form.gate_id} onChange={(event) => setForm({ ...form, gate_id: event.target.value })}><option value="">Select gate</option>{gates.filter((gate) => gate.is_active).map((gate) => <option key={gate.id} value={gate.id}>{gate.name} ({gate.code})</option>)}</select></label><label className="module-field"><span>Pass type <b>*</b></span><select required value={form.pass_type_id} onChange={(event) => setForm({ ...form, pass_type_id: event.target.value })}><option value="">Select pass type</option>{passTypes.filter((type) => type.is_active).map((type) => <option key={type.id} value={type.id}>{type.name}{type.requires_approval ? ' · Approval required' : ''}</option>)}</select></label><label className="module-field"><span>Valid from <b>*</b></span><input type="datetime-local" required value={form.valid_from} onChange={(event) => setForm({ ...form, valid_from: event.target.value })} /></label><label className="module-field"><span>Valid until <b>*</b></span><input type="datetime-local" required value={form.valid_until} onChange={(event) => setForm({ ...form, valid_until: event.target.value })} /></label></div>{formError && <div className="module-alert module-alert-error" role="alert">{formError}</div>}<footer className="module-modal-footer"><button type="button" className="module-button module-button-secondary" disabled={saving} onClick={() => { setModalOpen(false); setEditingPass(null); }}>Cancel</button><button type="submit" className="module-button module-button-primary" disabled={saving || !visitors.some((visitor) => !visitor.is_blacklisted) || !gates.some((gate) => gate.is_active) || !passTypes.some((type) => type.is_active)}>{saving ? 'Saving…' : editingPass ? 'Save draft changes' : 'Create visit draft'}</button></footer></form>
    </section></div>}
  </section>;
}
