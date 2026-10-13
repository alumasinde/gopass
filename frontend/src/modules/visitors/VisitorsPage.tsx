import { useCallback, useEffect, useMemo, useState } from 'react';
import type { FormEvent } from 'react';
import { tenantApiErrorMessage, tenantVisitorsApi } from '../../lib/tenant-api';
import type { Visitor, VisitorInput } from '../../lib/tenant-api';
import { useTenantAuth } from '../../contexts/TenantAuthContext';

type Filter = 'all' | 'active' | 'blacklisted';
const emptyForm: VisitorInput = { first_name: '', last_name: '', phone: '', email: '' };

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
  const [visitors, setVisitors] = useState<Visitor[]>([]);
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

  const load = useCallback(async (quiet = false) => {
    if (!canView) { setLoading(false); return; }
    if (quiet) setRefreshing(true); else setLoading(true);
    setError('');
    try {
      const rows = await tenantVisitorsApi.list();
      setVisitors(Array.isArray(rows) ? rows : []);
      setSelected((current) => current ? (rows.find((row) => row.id === current.id) ?? null) : null);
    } catch (err) {
      setError(tenantApiErrorMessage(err, 'Visitors could not be loaded.'));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [canView]);

  useEffect(() => { void load(); }, [load]);

  const filtered = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return visitors.filter((visitor) => {
      if (filter === 'active' && visitor.is_blacklisted) return false;
      if (filter === 'blacklisted' && !visitor.is_blacklisted) return false;
      if (!normalized) return true;
      return [visitor.first_name, visitor.last_name, visitor.phone, visitor.email, String(visitor.id)]
        .some((value) => String(value ?? '').toLowerCase().includes(normalized));
    });
  }, [visitors, query, filter]);

  const openCreate = () => {
    setEditing(null); setForm(emptyForm); setFormError(''); setModalOpen(true);
  };
  const openEdit = (visitor: Visitor) => {
    setEditing(visitor);
    setForm({ first_name: visitor.first_name ?? '', last_name: visitor.last_name ?? '', phone: visitor.phone ?? '', email: visitor.email ?? '' });
    setFormError(''); setModalOpen(true);
  };
  const closeModal = () => { if (!saving) { setModalOpen(false); setEditing(null); setFormError(''); } };
  const saveVisitor = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!form.first_name.trim() || !form.last_name.trim()) { setFormError('First name and last name are required.'); return; }
    setSaving(true); setFormError(''); setNotice('');
    try {
      const payload = { ...form, first_name: form.first_name.trim(), last_name: form.last_name.trim(), phone: form.phone.trim(), email: form.email.trim() };
      if (editing) {
        await tenantVisitorsApi.update(editing.id, payload);
        setNotice('Visitor details updated.');
      } else {
        const created = await tenantVisitorsApi.create(payload);
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
      if (visitor.is_blacklisted) await tenantVisitorsApi.unblacklist(visitor.id);
      else await tenantVisitorsApi.blacklist(visitor.id);
      setNotice(visitor.is_blacklisted ? 'Visitor removed from blacklist.' : 'Visitor added to blacklist.');
      await load(true);
    } catch (err) { setError(tenantApiErrorMessage(err, 'Visitor status could not be changed.')); }
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
          <div className="module-toolbar-controls"><label className="module-search"><span>⌕</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search name, phone, email…" aria-label="Search visitors" /></label><select className="module-select" value={filter} onChange={(event) => setFilter(event.target.value as Filter)} aria-label="Filter visitor status"><option value="all">All visitors</option><option value="active">Not blacklisted</option><option value="blacklisted">Blacklisted</option></select></div>
        </div>
        {loading ? <div className="module-table-loading"><span /><span /><span /><span /><span /></div> : filtered.length === 0 ? <div className="module-empty-state"><span className="module-empty-icon">♙</span><h3>{visitors.length ? 'No matching visitors' : 'No visitors registered yet'}</h3><p>{visitors.length ? 'Try another search or change the status filter.' : 'Register your first visitor to start building your directory.'}</p>{canCreate && !visitors.length && <button type="button" className="module-button module-button-primary" onClick={openCreate}>Register first visitor</button>}</div> : (
          <div className="module-table-scroll"><table className="module-table"><thead><tr><th>Visitor</th><th>Contact</th><th>Status</th><th>Visitor ID</th><th className="module-align-right">Actions</th></tr></thead><tbody>{filtered.map((visitor) => <tr key={visitor.id} className={selected?.id === visitor.id ? 'module-row-selected' : ''}>
            <td><button type="button" className="module-person-cell" onClick={() => setSelected(visitor)}><span className={`module-person-avatar${visitor.is_blacklisted ? ' module-avatar-muted' : ''}`}>{initials(visitor)}</span><span><strong>{displayName(visitor)}</strong><small>{visitor.email || 'No email provided'}</small></span></button></td>
            <td>{visitor.phone || <span className="module-muted">No phone</span>}</td>
            <td><span className={`module-status ${visitor.is_blacklisted ? 'module-status-danger' : 'module-status-success'}`}><i />{visitor.is_blacklisted ? 'Blacklisted' : 'Active'}</span></td>
            <td><span className="module-id">VIS-{String(visitor.id).padStart(5, '0')}</span></td>
            <td><div className="module-row-actions">{canUpdate && <button type="button" className="module-icon-button" aria-label={`Edit ${displayName(visitor)}`} title="Edit visitor" onClick={() => openEdit(visitor)}>✎</button>}{canBlacklist && <button type="button" className={`module-icon-button${visitor.is_blacklisted ? ' module-icon-button-positive' : ' module-icon-button-danger'}`} disabled={actionId === visitor.id} title={visitor.is_blacklisted ? 'Remove from blacklist' : 'Blacklist visitor'} aria-label={visitor.is_blacklisted ? 'Remove from blacklist' : 'Blacklist visitor'} onClick={() => void toggleBlacklist(visitor)}>{visitor.is_blacklisted ? '↶' : '⊘'}</button>}<button type="button" className="module-icon-button" title="View details" aria-label={`View ${displayName(visitor)}`} onClick={() => setSelected(visitor)}>→</button></div></td>
          </tr>)}</tbody></table></div>
        )}
      </div>

      {selected && <aside className="module-detail-panel"><div className="module-detail-avatar">{initials(selected)}</div><div className="module-detail-main"><div className="module-eyebrow">VISITOR PROFILE · VIS-{String(selected.id).padStart(5, '0')}</div><h2>{displayName(selected)}</h2><p>{selected.email || 'No email provided'} · {selected.phone || 'No phone provided'}</p></div><span className={`module-status ${selected.is_blacklisted ? 'module-status-danger' : 'module-status-success'}`}><i />{selected.is_blacklisted ? 'Blacklisted' : 'Active'}</span><div className="module-detail-actions">{canUpdate && <button className="module-button module-button-secondary" type="button" onClick={() => openEdit(selected)}>Edit details</button>}{canBlacklist && <button className="module-button module-button-secondary" type="button" onClick={() => void toggleBlacklist(selected)}>{selected.is_blacklisted ? 'Remove blacklist' : 'Blacklist visitor'}</button>}<button className="module-icon-button" type="button" aria-label="Close visitor details" onClick={() => setSelected(null)}>×</button></div></aside>}

      {modalOpen && <div className="module-modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) closeModal(); }}><section className="module-modal" role="dialog" aria-modal="true" aria-labelledby="visitor-form-title"><header className="module-modal-header"><div><div className="module-eyebrow">VISITOR DIRECTORY</div><h2 id="visitor-form-title">{editing ? 'Edit visitor' : 'Register visitor'}</h2><p>{editing ? 'Update the visitor contact details.' : 'Add a visitor record to your organization.'}</p></div><button type="button" className="module-icon-button" aria-label="Close" onClick={closeModal} disabled={saving}>×</button></header><form onSubmit={(event) => void saveVisitor(event)}><div className="module-form-grid"><label className="module-field"><span>First name <b>*</b></span><input autoFocus required maxLength={100} value={form.first_name} onChange={(event) => setForm({ ...form, first_name: event.target.value })} placeholder="e.g. Jane" /></label><label className="module-field"><span>Last name <b>*</b></span><input required maxLength={100} value={form.last_name} onChange={(event) => setForm({ ...form, last_name: event.target.value })} placeholder="e.g. Wanjiku" /></label><label className="module-field"><span>Phone number</span><input type="tel" maxLength={40} value={form.phone} onChange={(event) => setForm({ ...form, phone: event.target.value })} placeholder="e.g. 0712 345 678" /></label><label className="module-field"><span>Email address</span><input type="email" maxLength={254} value={form.email} onChange={(event) => setForm({ ...form, email: event.target.value })} placeholder="name@example.com" /></label></div>{formError && <div className="module-alert module-alert-error" role="alert">{formError}</div>}<footer className="module-modal-footer"><button type="button" className="module-button module-button-secondary" onClick={closeModal} disabled={saving}>Cancel</button><button type="submit" className="module-button module-button-primary" disabled={saving}>{saving ? 'Saving…' : editing ? 'Save changes' : 'Register visitor'}</button></footer></form></section></div>}
    </section>
  );
}
