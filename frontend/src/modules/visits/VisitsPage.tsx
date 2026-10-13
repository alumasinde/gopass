import { useCallback, useEffect, useMemo, useState } from 'react';
import type { FormEvent } from 'react';
import { useTenantAuth } from '../../contexts/TenantAuthContext';
import { tenantApiErrorMessage } from '../../lib/tenant-api';
import { visitsApi } from './api';
import type { TenantSite, TenantGate, Visitor, Visit, VisitInput, VisitUpdateInput } from '../../lib/tenant-api';

function formatDate(value?: string) {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('en-KE', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Africa/Nairobi' }).format(date);
}
function initials(visitor: Visitor) {
  return `${visitor.first_name?.[0] ?? ''}${visitor.last_name?.[0] ?? ''}`.toUpperCase() || 'V';
}
function displayName(visitor: Visitor) { return `${visitor.first_name ?? ''} ${visitor.last_name ?? ''}`.trim() || `Visitor #${visitor.id}`; }

type VisitForm = VisitInput;
const emptyForm: VisitForm = { visitor_id: 0, site_id: 0, gate_id: 0, direction: 'IN', purpose: '', host_name: '', host_department: '', notes: '' };

export default function VisitsPage() {
  const { user } = useTenantAuth();
  const permissions = new Set(user?.permissions ?? []);
  const canView = permissions.has('visits.view');
  const canCreate = permissions.has('visits.create');
  const canManage = permissions.has('visits.manage');
  const canViewVisitors = permissions.has('visitors.view');
  const canViewSites = permissions.has('sites.view');
  const canViewGates = permissions.has('gates.view');
  
  const [visits, setVisits] = useState<Visit[]>([]);
  const [visitors, setVisitors] = useState<Visitor[]>([]);
  const [sites, setSites] = useState<TenantSite[]>([]);
  const [gates, setGates] = useState<TenantGate[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [query, setQuery] = useState('');
  const [filter, setFilter] = useState<'all' | 'IN' | 'OUT'>('all');
  const [modalOpen, setModalOpen] = useState(false);
  const [editingVisit, setEditingVisit] = useState<Visit | null>(null);
  const [selectedVisit, setSelectedVisit] = useState<Visit | null>(null);
  const [form, setForm] = useState<VisitForm>(emptyForm);
  const [formError, setFormError] = useState('');
  const [saving, setSaving] = useState(false);
  const [actionId, setActionId] = useState<number | null>(null);

  const load = useCallback(async (quiet = false) => {
    if (!canView) { setLoading(false); return; }
    if (quiet) setRefreshing(true); else setLoading(true);
    setError('');
    const results = await Promise.allSettled([
      visitsApi.list(),
      canViewVisitors ? visitsApi.visitors() : Promise.resolve([]),
      canViewSites ? visitsApi.sites() : Promise.resolve([]),
      canViewGates ? visitsApi.gates() : Promise.resolve([]),
    ]);
    if (results[0].status === 'fulfilled') setVisits(results[0].value);
    else setError(tenantApiErrorMessage(results[0].reason, 'Visit records could not be loaded.'));
    if (results[1].status === 'fulfilled') setVisitors(results[1].value);
    if (results[2].status === 'fulfilled') setSites(results[2].value);
    if (results[3].status === 'fulfilled') setGates(results[3].value);
    setLoading(false); setRefreshing(false);
  }, [canView, canViewVisitors, canViewSites, canViewGates]);

  useEffect(() => { void load(); }, [load]);

  const visitorById = useMemo(() => new Map(visitors.map((visitor) => [visitor.id, visitor])), [visitors]);
  const siteById = useMemo(() => new Map(sites.map((site) => [site.id, site])), [sites]);
  const gateById = useMemo(() => new Map(gates.map((gate) => [gate.id, gate])), [gates]);
  
  const filtered = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return visits.filter((visit) => {
      if (filter !== 'all' && visit.direction !== filter) return false;
      const visitor = visitorById.get(visit.visitor_id);
      const site = siteById.get(visit.site_id);
      const gate = gateById.get(visit.gate_id);
      return !normalized || [String(visit.id), visitor ? `${visitor.first_name} ${visitor.last_name}` : '', visitor?.company_name, visitor?.id_number, site?.name, gate?.name, visit.direction, visit.purpose, visit.host_name]
        .some((value) => String(value ?? '').toLowerCase().includes(normalized));
    }).sort((a, b) => new Date(b.check_in_time).getTime() - new Date(a.check_in_time).getTime());
  }, [visits, filter, query, visitorById, siteById, gateById]);

  const openCreate = () => {
    setEditingVisit(null);
    setForm(emptyForm);
    setFormError('');
    setModalOpen(true);
  };

  const openEdit = (visit: Visit) => {
    setEditingVisit(visit);
    setForm({
      visitor_id: visit.visitor_id,
      site_id: visit.site_id,
      gate_id: visit.gate_id,
      direction: visit.direction,
      purpose: visit.purpose || '',
      host_name: visit.host_name || '',
      host_department: visit.host_department || '',
      notes: visit.notes || '',
    });
    setFormError('');
    setModalOpen(true);
  };

  const closeModal = () => {
    if (!saving) {
      setModalOpen(false);
      setEditingVisit(null);
      setFormError('');
    }
  };

  const saveVisit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!form.visitor_id || !form.site_id || !form.gate_id) {
      setFormError('Visitor, site, and gate are required.');
      return;
    }
    setSaving(true);
    setFormError('');
    setNotice('');
    try {
      if (editingVisit) {
        const updateInput: VisitUpdateInput = {
          purpose: form.purpose || undefined,
          host_name: form.host_name || undefined,
          host_department: form.host_department || undefined,
          notes: form.notes || undefined,
        };
        await visitsApi.update(editingVisit.id, updateInput);
        setNotice('Visit details updated.');
      } else {
        await visitsApi.create(form);
        setNotice('Visit recorded successfully.');
      }
      setModalOpen(false);
      setEditingVisit(null);
      setForm(emptyForm);
      await load(true);
    } catch (err) {
      setFormError(tenantApiErrorMessage(err, 'Visit could not be saved. Check your permissions and try again.'));
    } finally {
      setSaving(false);
    }
  };

  const checkOutVisit = async (visit: Visit) => {
    if (!window.confirm(`Check out this visit? This will record the check-out time.`)) return;
    setActionId(visit.id);
    setError('');
    setNotice('');
    try {
      const now = new Date().toISOString();
      await visitsApi.update(visit.id, { check_out_time: now });
      setNotice('Visit checked out successfully.');
      await load(true);
    } catch (err) {
      setError(tenantApiErrorMessage(err, 'Visit could not be checked out.'));
    } finally {
      setActionId(null);
    }
  };

  const deleteVisit = async (visit: Visit) => {
    if (!window.confirm(`Delete this visit record? This action cannot be undone.`)) return;
    setActionId(visit.id);
    setError('');
    setNotice('');
    try {
      await visitsApi.delete(visit.id);
      setNotice('Visit record deleted.');
      setSelectedVisit(null);
      await load(true);
    } catch (err) {
      setError(tenantApiErrorMessage(err, 'Visit could not be deleted.'));
    } finally {
      setActionId(null);
    }
  };

  if (!canView) return <section className="module-page"><div className="module-access-denied"><span className="module-icon">⌁</span><h1>Visits access required</h1><p>Your account does not have the <code>visits.view</code> permission. Ask your organization administrator to update your role.</p></div></section>;

  const checkedIn = visits.filter((visit) => !visit.check_out_time).length;
  const checkedOut = visits.filter((visit) => visit.check_out_time).length;
  const blacklisted = visits.filter((visit) => visit.is_blacklisted).length;

  return (
    <section className="module-page">
      <header className="module-page-header">
        <div>
          <div className="module-eyebrow">WORKSPACE / VISIT LOGS</div>
          <h1>Visits</h1>
          <p>Record and track visitor check-ins and check-outs independently of gatepasses.</p>
        </div>
        <div className="module-header-actions">
          <button type="button" className="module-button module-button-secondary" onClick={() => void load(true)} disabled={refreshing}>
            {refreshing ? 'Refreshing…' : '↻ Refresh'}
          </button>
          {canCreate && <button type="button" className="module-button module-button-primary" onClick={openCreate}>＋ Record visit</button>}
        </div>
      </header>

      <div className="module-metric-row">
        <article className="module-metric-card"><span className="module-metric-icon module-tone-green">✓</span><div><small>Checked in</small><strong>{checkedIn}</strong></div></article>
        <article className="module-metric-card"><span className="module-metric-icon module-tone-blue">←</span><div><small>Checked out</small><strong>{checkedOut}</strong></div></article>
        <article className="module-metric-card"><span className="module-metric-icon module-tone-red">!</span><div><small>Blacklisted</small><strong>{blacklisted}</strong></div></article>
      </div>

      {notice && <div className="module-alert module-alert-success" role="status"><span>✓</span>{notice}<button type="button" aria-label="Dismiss notice" onClick={() => setNotice('')}>×</button></div>}
      {error && <div className="module-alert module-alert-error" role="alert"><span>!</span><div>{error}</div><button type="button" onClick={() => void load()} className="module-alert-action">Retry</button></div>}

      <div className="module-panel">
        <div className="module-toolbar">
          <div><h2>Visit log</h2><p>Showing {filtered.length} of {visits.length} records</p></div>
          <div className="module-toolbar-controls">
            <label className="module-search"><span>⌕</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search visitor, site, gate, host…" aria-label="Search visits" /></label>
            <select className="module-select" value={filter} onChange={(event) => setFilter(event.target.value as 'all' | 'IN' | 'OUT')} aria-label="Filter visit direction">
              <option value="all">All directions</option>
              <option value="IN">Check-ins only</option>
              <option value="OUT">Check-outs only</option>
            </select>
          </div>
        </div>
        {loading ? <div className="module-table-loading"><span /><span /><span /><span /><span /></div> : filtered.length === 0 ? (
          <div className="module-empty-state">
            <span className="module-empty-icon">◷</span>
            <h3>{visits.length ? 'No matching visits' : 'No visits recorded yet'}</h3>
            <p>{visits.length ? 'Try another search or change the filter.' : 'Record your first visit to start tracking visitor access.'}</p>
            {canCreate && !visits.length && <button type="button" className="module-button module-button-primary" onClick={openCreate}>Record first visit</button>}
          </div>
        ) : (
          <div className="module-table-scroll">
            <table className="module-table">
              <thead>
                <tr>
                  <th>Visit</th>
                  <th>Visitor</th>
                  <th>Location</th>
                  <th>Direction</th>
                  <th>Check-in</th>
                  <th>Check-out</th>
                  <th>Status</th>
                  <th className="module-align-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((visit) => {
                  const visitor = visitorById.get(visit.visitor_id);
                  const site = siteById.get(visit.site_id);
                  const gate = gateById.get(visit.gate_id);
                  return (
                    <tr key={visit.id} className={selectedVisit?.id === visit.id ? 'module-row-selected' : ''}>
                      <td><strong className="module-primary-text">VISIT-{String(visit.id).padStart(5, '0')}</strong><small className="module-table-subtext">Record #{visit.id}</small></td>
                      <td>
                        <div className="module-inline-person">
                          <span className="module-person-avatar">{visitor ? initials(visitor) : 'V'}</span>
                          <span>
                            <strong>{visitor ? displayName(visitor) : `Visitor #${visit.visitor_id}`}</strong>
                            <small>{visitor?.company_name || 'Company not provided'}</small>
                            <small>{visitor?.phone || visitor?.email || 'Contact not available'}</small>
                          </span>
                        </div>
                      </td>
                      <td>
                        <strong>{site?.name || `Site #${visit.site_id}`}</strong>
                        <small className="module-table-subtext">{gate?.name || `Gate #${visit.gate_id}`}</small>
                      </td>
                      <td><span className={`module-status ${visit.direction === 'IN' ? 'module-status-success' : 'module-status-neutral'}`}><i />{visit.direction}</span></td>
                      <td><strong>{formatDate(visit.check_in_time)}</strong></td>
                      <td>{visit.check_out_time ? <strong>{formatDate(visit.check_out_time)}</strong> : <span className="module-muted">Not checked out</span>}</td>
                      <td>
                        {visit.is_blacklisted ? <span className="module-status module-status-danger"><i />Blacklisted</span> : 
                         visit.check_out_time ? <span className="module-status module-status-neutral"><i />Completed</span> : 
                         <span className="module-status module-status-success"><i />Active</span>}
                      </td>
                      <td>
                        <div className="module-row-actions">
                          {canManage && !visit.check_out_time && <button type="button" className="module-button module-button-small module-button-secondary" disabled={actionId === visit.id} onClick={() => void checkOutVisit(visit)}>Check out</button>}
                          {canManage && <button type="button" className="module-icon-button" aria-label={`Edit visit #${visit.id}`} title="Edit visit" onClick={() => openEdit(visit)}>✎</button>}
                          {canManage && <button type="button" className="module-icon-button" aria-label={`View visit #${visit.id}`} title="View details" onClick={() => setSelectedVisit(visit)}>→</button>}
                          {canManage && <button type="button" className="module-icon-button module-icon-button-danger" aria-label={`Delete visit #${visit.id}`} title="Delete visit" disabled={actionId === visit.id} onClick={() => void deleteVisit(visit)}>⌫</button>}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {selectedVisit && (
        <aside className="module-detail-panel">
          <div className="module-detail-avatar">V</div>
          <div className="module-detail-main">
            <div className="module-eyebrow">VISIT RECORD · VISIT-{String(selectedVisit.id).padStart(5, '0')}</div>
            <h2>Visit #{selectedVisit.id}</h2>
            <p>
              {selectedVisit.direction === 'IN' ? 'Check-in' : 'Check-out'} at {formatDate(selectedVisit.check_in_time)}
              {selectedVisit.check_out_time && ` · Checked out at ${formatDate(selectedVisit.check_out_time)}`}
            </p>
            {selectedVisit.purpose && <p><strong>Purpose:</strong> {selectedVisit.purpose}</p>}
            {selectedVisit.host_name && <p><strong>Host:</strong> {selectedVisit.host_name}{selectedVisit.host_department && ` · ${selectedVisit.host_department}`}</p>}
            {selectedVisit.notes && <p><strong>Notes:</strong> {selectedVisit.notes}</p>}
            {selectedVisit.is_blacklisted && <p className="module-alert module-alert-error"><strong>Blacklisted</strong>{selectedVisit.blacklist_reason && `: ${selectedVisit.blacklist_reason}`}</p>}
          </div>
          <div className="module-detail-actions">
            <button className="module-icon-button" type="button" aria-label="Close visit details" onClick={() => setSelectedVisit(null)}>×</button>
          </div>
        </aside>
      )}

      {modalOpen && (
        <div className="module-modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) closeModal(); }}>
          <section className="module-modal" role="dialog" aria-modal="true" aria-labelledby="visit-form-title">
            <header className="module-modal-header">
              <div>
                <div className="module-eyebrow">VISIT LOGS</div>
                <h2 id="visit-form-title">{editingVisit ? 'Edit visit' : 'Record visit'}</h2>
                <p>{editingVisit ? 'Update visit details.' : 'Record a new visitor check-in or check-out.'}</p>
              </div>
              <button type="button" className="module-icon-button" aria-label="Close" onClick={closeModal} disabled={saving}>×</button>
            </header>
            <form onSubmit={(event) => void saveVisit(event)}>
              <div className="module-form-grid">
                <label className="module-field module-field-full">
                  <span>Visitor <b>*</b></span>
                  <select required value={form.visitor_id || ''} onChange={(event) => setForm({ ...form, visitor_id: Number(event.target.value) })}>
                    <option value="">Select visitor</option>
                    {visitors.filter((visitor) => !visitor.is_blacklisted).map((visitor) => (
                      <option key={visitor.id} value={visitor.id}>
                        {visitor.first_name} {visitor.last_name}{visitor.company_name ? ` · ${visitor.company_name}` : ''}{visitor.id_number ? ` · ${visitor.id_type_name || 'ID'} ${visitor.id_number}` : ''}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="module-field">
                  <span>Site <b>*</b></span>
                  <select required value={form.site_id || ''} onChange={(event) => setForm({ ...form, site_id: Number(event.target.value) })}>
                    <option value="">Select site</option>
                    {sites.filter((site) => site.is_active).map((site) => (
                      <option key={site.id} value={site.id}>{site.name} ({site.code})</option>
                    ))}
                  </select>
                </label>
                <label className="module-field">
                  <span>Gate <b>*</b></span>
                  <select required value={form.gate_id || ''} onChange={(event) => setForm({ ...form, gate_id: Number(event.target.value) })}>
                    <option value="">Select gate</option>
                    {gates.filter((gate) => gate.is_active).map((gate) => (
                      <option key={gate.id} value={gate.id}>{gate.name} ({gate.code})</option>
                    ))}
                  </select>
                </label>
                <label className="module-field">
                  <span>Direction <b>*</b></span>
                  <select required value={form.direction} onChange={(event) => setForm({ ...form, direction: event.target.value as 'IN' | 'OUT' })} disabled={!!editingVisit}>
                    <option value="IN">Check-in (IN)</option>
                    <option value="OUT">Check-out (OUT)</option>
                  </select>
                </label>
                <label className="module-field module-field-full">
                  <span>Purpose</span>
                  <input value={form.purpose} onChange={(event) => setForm({ ...form, purpose: event.target.value })} placeholder="e.g. Meeting, Delivery, Interview" />
                </label>
                <label className="module-field">
                  <span>Host name</span>
                  <input value={form.host_name} onChange={(event) => setForm({ ...form, host_name: event.target.value })} placeholder="e.g. John Doe" />
                </label>
                <label className="module-field">
                  <span>Host department</span>
                  <input value={form.host_department} onChange={(event) => setForm({ ...form, host_department: event.target.value })} placeholder="e.g. HR, IT, Finance" />
                </label>
                <label className="module-field module-field-full">
                  <span>Notes</span>
                  <textarea value={form.notes} onChange={(event) => setForm({ ...form, notes: event.target.value })} placeholder="Additional notes about this visit..." rows={3} />
                </label>
              </div>
              {formError && <div className="module-alert module-alert-error"><span>!</span><div>{formError}</div></div>}
              <div className="module-modal-footer">
                <button type="button" className="module-button module-button-secondary" onClick={closeModal} disabled={saving}>Cancel</button>
                <button type="submit" className="module-button module-button-primary" disabled={saving}>{saving ? 'Saving…' : editingVisit ? 'Update visit' : 'Record visit'}</button>
              </div>
            </form>
          </section>
        </div>
      )}
    </section>
  );
}
