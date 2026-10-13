import { useCallback, useEffect, useMemo, useState } from 'react';
import type { FormEvent } from 'react';
import { tenantApiErrorMessage } from '../../lib/tenant-api';
import { visitorsApi } from './api';
import type { Visitor, VisitorInput } from '../../lib/tenant-api';
import { useTenantAuth } from '../../contexts/TenantAuthContext';
import { identificationTypesApi } from '../identification-types/api';
import type { IdentificationType } from '../../lib/tenant-api';

type Filter = 'all' | 'active' | 'blacklisted';
const emptyForm: VisitorInput = { first_name: '', last_name: '', company_name: '', phone: '', email: '', id_type_id: null, id_number: '', id_expiry_date: '' };

function initials(visitor: Visitor) {
  return `${visitor.first_name?.[0] ?? ''}${visitor.last_name?.[0] ?? ''}`.toUpperCase() || 'V';
}
function displayName(visitor: Visitor) { return `${visitor.first_name ?? ''} ${visitor.last_name ?? ''}`.trim() || `Visitor #${visitor.id}`; }

export default function VisitorsPage() {
  const { user } = useTenantAuth();
  const permissions = new Set(user?.permissions ?? []);
  const canView = permissions.has('visitors.view');
  const canCreate = permissions.has('visitors.register');
  const canUpdate = permissions.has('visitors.update');
  const canBlacklist = permissions.has('visitors.blacklist');
  const canArchive = permissions.has('visitors.archive');
  const canViewIDTypes = permissions.has('identification_types.view');
  const [visitors, setVisitors] = useState<Visitor[]>([]);
  const [identificationTypes, setIdentificationTypes] = useState<IdentificationType[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState('');
  const [query, setQuery] = useState('');
  const [filter, setFilter] = useState<Filter>('all');
  const [selected, setSelected] = useState<Visitor | null>(null);
  const [editing, setEditing] = useState<Visitor | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [form, setForm] = useState<VisitorInput>(emptyForm);
  const [saving, setSaving] = useState(false);
  const [actionId, setActionId] = useState<number | null>(null);
  const [formError, setFormError] = useState('');
  const [notice, setNotice] = useState('');
  const riskPreview = useMemo(() => {
    let score = 0;
    const factors: string[] = [];
    if (!form.phone.trim()) { score += 20; factors.push('No phone number (+20)'); }
    if (!form.email.trim()) { score += 15; factors.push('No email address (+15)'); }
    if (!form.id_type_id || !form.id_number.trim()) { score += 30; factors.push('Identification type or number missing (+30)'); }
    if (form.id_expiry_date && form.id_expiry_date < new Date().toISOString().slice(0, 10)) { score += 15; factors.push('Identification document expired (+15)'); }
    if (!form.company_name.trim()) { score += 5; factors.push('Company/affiliation not supplied (+5)'); }
    if (editing?.is_blacklisted) { score = 100; factors.push('Active blacklist status'); }
    score = Math.min(100, score);
    const level = editing?.is_blacklisted || score >= 75 ? 'CRITICAL' : score >= 50 ? 'HIGH' : score >= 25 ? 'MEDIUM' : 'LOW';
    if (!factors.length) factors.push('No configured risk factors detected');
    return { score, level, factors };
  }, [form, editing]);

  const load = useCallback(async (quiet = false) => {
    if (!canView) { setLoading(false); return; }
    if (quiet) setRefreshing(true); else setLoading(true);
    setError('');
    try {
      const [visitorResult, typeResult] = await Promise.allSettled([
        visitorsApi.list(),
        canViewIDTypes ? identificationTypesApi.list() : Promise.resolve([] as IdentificationType[]),
      ]);
      if (visitorResult.status === 'rejected') throw visitorResult.reason;
      const rows = Array.isArray(visitorResult.value) ? visitorResult.value : [];
      setVisitors(rows);
      setSelected((current) => current ? (rows.find((row) => row.id === current.id) ?? null) : null);
      if (typeResult.status === 'fulfilled') setIdentificationTypes(typeResult.value);
      else setIdentificationTypes([]);
    } catch (err) {
      setError(tenantApiErrorMessage(err, 'Visitors could not be loaded.'));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [canView, canViewIDTypes]);

  useEffect(() => { void load(); }, [load]);

  const filtered = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return visitors.filter((visitor) => {
      if (filter === 'active' && visitor.is_blacklisted) return false;
      if (filter === 'blacklisted' && !visitor.is_blacklisted) return false;
      if (!normalized) return true;
      return [visitor.first_name, visitor.last_name, visitor.company_name, visitor.phone, visitor.email, visitor.id_number, visitor.id_type_name, visitor.risk_level, String(visitor.id)]
        .some((value) => String(value ?? '').toLowerCase().includes(normalized));
    });
  }, [visitors, query, filter]);

  const openCreate = () => {
    setEditing(null); setForm(emptyForm); setFormError(''); setModalOpen(true);
  };
  const openEdit = (visitor: Visitor) => {
    setEditing(visitor);
    setForm({ first_name: visitor.first_name ?? '', last_name: visitor.last_name ?? '', company_name: visitor.company_name ?? '', phone: visitor.phone ?? '', email: visitor.email ?? '', id_type_id: visitor.id_type_id ?? null, id_number: visitor.id_number ?? '', id_expiry_date: visitor.id_expiry_date ?? '' });
    setFormError(''); setModalOpen(true);
  };
  const closeModal = () => { if (!saving) { setModalOpen(false); setEditing(null); setFormError(''); } };
  const saveVisitor = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!form.first_name.trim() || !form.last_name.trim()) { setFormError('First name and last name are required.'); return; }
    setSaving(true); setFormError(''); setNotice('');
    try {
      const payload = { ...form, first_name: form.first_name.trim(), last_name: form.last_name.trim(), company_name: form.company_name.trim(), phone: form.phone.trim(), email: form.email.trim(), id_number: form.id_number.trim().toUpperCase() };
      if (editing) {
        await visitorsApi.update(editing.id, payload);
        setNotice('Visitor details updated.');
      } else {
        const created = await visitorsApi.create(payload);
        setSelected(created);
        setNotice('Visitor registered successfully.');
      }
      setModalOpen(false); setEditing(null);
      await load(true);
    } catch (err) {
      setFormError(tenantApiErrorMessage(err, 'Visitor could not be saved. Check your permissions and try again.'));
    } finally { setSaving(false); }
  };
  const toggleBlacklist = async (visitor: Visitor) => {
    const confirmed = window.confirm(visitor.is_blacklisted ? `Remove ${displayName(visitor)} from the blacklist?` : `Blacklist ${displayName(visitor)}? They may not be allowed to receive gatepasses.`);
    if (!confirmed) return;
    setActionId(visitor.id); setError(''); setNotice('');
    try {
      if (visitor.is_blacklisted) await visitorsApi.unblacklist(visitor.id);
      else await visitorsApi.blacklist(visitor.id);
      setNotice(visitor.is_blacklisted ? 'Visitor removed from blacklist.' : 'Visitor added to blacklist.');
      await load(true);
    } catch (err) { setError(tenantApiErrorMessage(err, 'Visitor status could not be changed.')); }
    finally { setActionId(null); }
  };


  const archiveVisitor = async (visitor: Visitor) => {
    if (!window.confirm(`Archive ${displayName(visitor)}? Historical gatepass records will be retained.`)) return;
    setActionId(visitor.id); setError(''); setNotice('');
    try { await visitorsApi.archive(visitor.id); setNotice('Visitor archived. Historical access records are retained.'); setSelected(current => current?.id === visitor.id ? null : current); await load(true); }
    catch (err) { setError(tenantApiErrorMessage(err, 'Visitor could not be archived.')); }
    finally { setActionId(null); }
  };

  if (!canView) return <section className="module-page"><div className="module-access-denied"><span className="module-icon">⌁</span><h1>Visitors access required</h1><p>Your account does not have the <code>visitors.view</code> permission. Ask your organization administrator to update your role.</p></div></section>;

  return (
    <section className="module-page">
      <header className="module-page-header">
        <div><div className="module-eyebrow">WORKSPACE / DIRECTORY</div><h1>Visitors</h1><p>Register people who visit your organization and manage their visitor records.</p></div>
        <div className="module-header-actions"><button type="button" className="module-button module-button-secondary" onClick={() => void load(true)} disabled={refreshing}>{refreshing ? 'Refreshing…' : '↻ Refresh'}</button>{canCreate && <button type="button" className="module-button module-button-primary" onClick={openCreate}>＋ Register visitor</button>}</div>
      </header>

      <div className="module-metric-row">
        <article className="module-metric-card"><span className="module-metric-icon module-tone-teal">♙</span><div><small>Total visitors</small><strong>{visitors.length}</strong></div></article>
        <article className="module-metric-card"><span className="module-metric-icon module-tone-green">✓</span><div><small>Active records</small><strong>{visitors.filter((visitor) => !visitor.is_blacklisted).length}</strong></div></article>
        <article className="module-metric-card"><span className="module-metric-icon module-tone-red">!</span><div><small>Blacklisted</small><strong>{visitors.filter((visitor) => visitor.is_blacklisted).length}</strong></div></article>
      </div>

      {notice && <div className="module-alert module-alert-success" role="status"><span>✓</span>{notice}<button type="button" aria-label="Dismiss notice" onClick={() => setNotice('')}>×</button></div>}
      {error && <div className="module-alert module-alert-error" role="alert"><span>!</span><div>{error}</div><button type="button" onClick={() => void load()} className="module-alert-action">Retry</button></div>}

      <div className="module-panel">
        <div className="module-toolbar">
          <div><h2>Visitor directory</h2><p>Showing {filtered.length} of {visitors.length} records</p></div>
          <div className="module-toolbar-controls"><label className="module-search"><span>⌕</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search name, company, ID, phone, email…" aria-label="Search visitors" /></label><select className="module-select" value={filter} onChange={(event) => setFilter(event.target.value as Filter)} aria-label="Filter visitor status"><option value="all">All visitors</option><option value="active">Not blacklisted</option><option value="blacklisted">Blacklisted</option></select></div>
        </div>
        {loading ? <div className="module-table-loading"><span /><span /><span /><span /><span /></div> : filtered.length === 0 ? <div className="module-empty-state"><span className="module-empty-icon">♙</span><h3>{visitors.length ? 'No matching visitors' : 'No visitors registered yet'}</h3><p>{visitors.length ? 'Try another search or change the status filter.' : 'Register your first visitor to start building your directory.'}</p>{canCreate && !visitors.length && <button type="button" className="module-button module-button-primary" onClick={openCreate}>Register first visitor</button>}</div> : (
          <div className="module-table-scroll"><table className="module-table"><thead><tr><th>Visitor</th><th>Company</th><th>Identification</th><th>Contact</th><th>Risk</th><th>Status</th><th>Record</th><th className="module-align-right">Actions</th></tr></thead><tbody>{filtered.map((visitor) => <tr key={visitor.id} className={selected?.id === visitor.id ? 'module-row-selected' : ''}>
            <td><button type="button" className="module-person-cell" onClick={() => setSelected(visitor)}><span className={`module-person-avatar${visitor.is_blacklisted ? ' module-avatar-muted' : ''}`}>{initials(visitor)}</span><span><strong>{displayName(visitor)}</strong><small>{visitor.email || 'No email provided'}</small></span></button></td>
            <td>{visitor.company_name || <span className="module-muted">Not provided</span>}</td>
            <td><strong>{visitor.id_type_name || 'No ID type'}</strong><small className="module-table-subtext">{visitor.id_number || 'No ID number'}</small></td>
            <td><div>{visitor.phone || <span className="module-muted">No phone</span>}</div><small className="module-table-subtext">{visitor.email || 'No email'}</small></td>
            <td><span className={`module-status ${visitor.risk_level === 'CRITICAL' || visitor.risk_level === 'HIGH' ? 'module-status-danger' : visitor.risk_level === 'MEDIUM' ? 'module-status-warning' : 'module-status-success'}`}><i />{visitor.risk_level || 'LOW'} · {visitor.risk_score ?? 0}</span><small className="module-table-subtext">{(visitor.risk_factors ?? []).length} factor(s)</small></td>
            <td><span className={`module-status ${visitor.is_blacklisted ? 'module-status-danger' : 'module-status-success'}`}><i />{visitor.is_blacklisted ? 'Blacklisted' : 'Active'}</span></td>
            <td><span className="module-id">VIS-{String(visitor.id).padStart(5, '0')}</span></td>
            <td><div className="module-row-actions">{canUpdate && <button type="button" className="module-icon-button" aria-label={`Edit ${displayName(visitor)}`} title="Edit visitor" onClick={() => openEdit(visitor)}>✎</button>}{canBlacklist && <button type="button" className={`module-icon-button${visitor.is_blacklisted ? ' module-icon-button-positive' : ' module-icon-button-danger'}`} disabled={actionId === visitor.id} title={visitor.is_blacklisted ? 'Remove from blacklist' : 'Blacklist visitor'} aria-label={visitor.is_blacklisted ? 'Remove from blacklist' : 'Blacklist visitor'} onClick={() => void toggleBlacklist(visitor)}>{visitor.is_blacklisted ? '↶' : '⊘'}</button>}{canArchive && <button type="button" className="module-icon-button module-icon-button-danger" disabled={actionId === visitor.id} title="Archive visitor" aria-label="Archive visitor" onClick={() => void archiveVisitor(visitor)}>⌫</button>}<button type="button" className="module-icon-button" title="View details" aria-label={`View ${displayName(visitor)}`} onClick={() => setSelected(visitor)}>→</button></div></td>
          </tr>)}</tbody></table></div>
        )}
      </div>

      {selected && <aside className="module-detail-panel"><div className="module-detail-avatar">{initials(selected)}</div><div className="module-detail-main"><div className="module-eyebrow">VISITOR PROFILE · VIS-{String(selected.id).padStart(5, '0')}</div><h2>{displayName(selected)}</h2><p>{selected.company_name || 'No company provided'} · {selected.id_type_name || 'No ID type'} {selected.id_number ? `· ${selected.id_number}` : ''}</p><p>{selected.email || 'No email'} · {selected.phone || 'No phone'}</p><div className="module-risk-detail"><strong>Risk score: {selected.risk_score ?? 0}/100 · {selected.risk_level || 'LOW'}</strong><ul>{(selected.risk_factors ?? []).map((factor) => <li key={factor}>{factor}</li>)}</ul></div></div><span className={`module-status ${selected.is_blacklisted ? 'module-status-danger' : 'module-status-success'}`}><i />{selected.is_blacklisted ? 'Blacklisted' : 'Active'}</span><div className="module-detail-actions">{canUpdate && <button className="module-button module-button-secondary" type="button" onClick={() => openEdit(selected)}>Edit details</button>}{canBlacklist && <button className="module-button module-button-secondary" type="button" onClick={() => void toggleBlacklist(selected)}>{selected.is_blacklisted ? 'Remove blacklist' : 'Blacklist visitor'}</button>}<button className="module-icon-button" type="button" aria-label="Close visitor details" onClick={() => setSelected(null)}>×</button></div></aside>}

      {modalOpen && <div className="module-modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) closeModal(); }}><section className="module-modal" role="dialog" aria-modal="true" aria-labelledby="visitor-form-title"><header className="module-modal-header"><div><div className="module-eyebrow">VISITOR DIRECTORY</div><h2 id="visitor-form-title">{editing ? 'Edit visitor' : 'Register visitor'}</h2><p>{editing ? 'Update the visitor contact details.' : 'Add a visitor record to your organization.'}</p></div><button type="button" className="module-icon-button" aria-label="Close" onClick={closeModal} disabled={saving}>×</button></header><form onSubmit={(event) => void saveVisitor(event)}><div className="module-form-grid"><label className="module-field"><span>First name <b>*</b></span><input autoFocus required maxLength={100} value={form.first_name} onChange={(event) => setForm({ ...form, first_name: event.target.value })} placeholder="e.g. Jane" /></label><label className="module-field"><span>Last name <b>*</b></span><input required maxLength={100} value={form.last_name} onChange={(event) => setForm({ ...form, last_name: event.target.value })} placeholder="e.g. Wanjiku" /></label><label className="module-field"><span>Company / organization</span><input maxLength={180} value={form.company_name} onChange={(event) => setForm({ ...form, company_name: event.target.value })} placeholder="e.g. Acme Limited" /></label><label className="module-field"><span>Phone number</span><input type="tel" maxLength={40} value={form.phone} onChange={(event) => setForm({ ...form, phone: event.target.value })} placeholder="e.g. 0712 345 678" /></label><label className="module-field"><span>Email address</span><input type="email" maxLength={254} value={form.email} onChange={(event) => setForm({ ...form, email: event.target.value })} placeholder="name@example.com" /></label><label className="module-field"><span>Identification type</span><select value={form.id_type_id ?? ''} onChange={(event) => setForm({ ...form, id_type_id: event.target.value ? Number(event.target.value) : null })}><option value="">No identification selected</option>{identificationTypes.filter((item) => item.is_active).map((item) => <option key={item.id} value={item.id}>{item.name}{item.is_default ? ' · Default' : ''}</option>)}</select></label><label className="module-field"><span>Identification number</span><input maxLength={100} value={form.id_number} onChange={(event) => setForm({ ...form, id_number: event.target.value.toUpperCase() })} placeholder="Document number" /></label><label className="module-field"><span>Identification expiry date</span><input type="date" value={form.id_expiry_date} onChange={(event) => setForm({ ...form, id_expiry_date: event.target.value })} /></label></div><div className={`module-risk-note module-risk-note-${riskPreview.level.toLowerCase()}`}><div className="module-risk-note-heading"><strong>Live risk preview</strong><span>{riskPreview.level} · {riskPreview.score}/100</span></div><ul>{riskPreview.factors.map((factor) => <li key={factor}>{factor}</li>)}</ul><small>The backend recalculates the authoritative score after saving and checks active blacklist matches before issuing access.</small></div>{formError && <div className="module-alert module-alert-error" role="alert">{formError}</div>}<footer className="module-modal-footer"><button type="button" className="module-button module-button-secondary" onClick={closeModal} disabled={saving}>Cancel</button><button type="submit" className="module-button module-button-primary" disabled={saving}>{saving ? 'Saving…' : editing ? 'Save changes' : 'Register visitor'}</button></footer></form></section></div>}
    </section>
  );
}
