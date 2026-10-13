import { useCallback, useEffect, useMemo, useState } from 'react';
import type { FormEvent } from 'react';
import { useTenantAuth } from '../../contexts/TenantAuthContext';
import { tenantApiErrorMessage } from '../../lib/tenant-api';
import type { IdentificationType, IdentificationTypeInput } from '../../lib/tenant-api';
import { identificationTypesApi } from './api';

const blank: IdentificationTypeInput = { code: '', name: '', description: '', is_active: true, is_default: false, validation_regex: '', validation_example: '' };

export default function IdentificationTypesPage() {
  const { user } = useTenantAuth();
  const permissions = new Set(user?.permissions ?? []);
  const canView = permissions.has('identification_types.view');
  const canManage = permissions.has('identification_types.manage');
  const [items, setItems] = useState<IdentificationType[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [query, setQuery] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [editing, setEditing] = useState<IdentificationType | null>(null);
  const [modal, setModal] = useState(false);
  const [form, setForm] = useState<IdentificationTypeInput>(blank);
  const [formError, setFormError] = useState('');

  const load = useCallback(async () => {
    if (!canView) { setLoading(false); return; }
    setLoading(true); setError('');
    try { const result = await identificationTypesApi.list(); setItems(Array.isArray(result) ? result : []); }
    catch (err) { setError(tenantApiErrorMessage(err, 'Identification types could not be loaded.')); }
    finally { setLoading(false); }
  }, [canView]);
  useEffect(() => { void load(); }, [load]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return items.filter((item) => !q || [item.name,item.code,item.description].some((v) => String(v ?? '').toLowerCase().includes(q)));
  }, [items,query]);

  const openCreate = () => { setEditing(null); setForm(blank); setFormError(''); setModal(true); };
  const openEdit = (item: IdentificationType) => {
    setEditing(item);
    setForm({ code: item.code, name: item.name, description: item.description ?? '', is_active: item.is_active, is_default: item.is_default, validation_regex: item.validation_regex ?? '', validation_example: item.validation_example ?? '' });
    setFormError(''); setModal(true);
  };
  const save = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault(); setFormError('');
    if (!form.name.trim() || (!editing && !/^[A-Za-z][A-Za-z0-9_]{1,49}$/.test(form.code.trim()))) {
      setFormError('Enter a name and a valid code (letters, numbers and underscores; start with a letter).'); return;
    }
    setBusy(true); setNotice('');
    try {
      if (editing) {
        const update = {
          name: form.name,
          description: form.description,
          is_active: form.is_active,
          is_default: form.is_default,
          validation_regex: form.validation_regex,
          validation_example: form.validation_example,
        };
        await identificationTypesApi.update(editing.id, update);
        setNotice('Identification type updated.');
      } else {
        await identificationTypesApi.create({ ...form, code: form.code.trim().toUpperCase() });
        setNotice('Identification type created.');
      }
      setModal(false); setEditing(null); await load();
    } catch (err) { setFormError(tenantApiErrorMessage(err, 'Could not save identification type.')); }
    finally { setBusy(false); }
  };
  const remove = async (item: IdentificationType) => {
    if (item.is_default) { setError('Default identification types cannot be deleted. Deactivate them instead.'); return; }
    if (!window.confirm(`Delete ${item.name}? Types assigned to visitor records cannot be deleted.`)) return;
    setError(''); setNotice('');
    try { await identificationTypesApi.delete(item.id); setNotice('Identification type deleted.'); await load(); }
    catch (err) { setError(tenantApiErrorMessage(err, 'Could not delete this type. If it is in use, deactivate it instead.')); }
  };

  if (!canView) return <section className="crud-page"><div className="crud-state"><strong>Identification type access required</strong><span>Your role needs identification_types.view.</span></div></section>;

  return <section className="crud-page">
    <header className="crud-page-header"><div><div className="crud-eyebrow">ADMINISTRATION / VISITOR IDENTITY</div><h1>Identification types</h1><p>Configure the identity documents your organization accepts for visitors.</p></div>{canManage && <button className="crud-button crud-button-primary" type="button" onClick={openCreate}>＋ Add identification type</button>}</header>
    <div className="crud-risk-note"><strong>Validation note</strong><span>Validation rules are configurable per organization. Use permissive patterns when formats vary; a document format check is not proof of identity.</span></div>
    {notice && <div className="crud-notice" role="status">{notice}<button type="button" onClick={() => setNotice('')}>×</button></div>}
    {error && <div className="crud-inline-error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></div>}
    <div className="crud-panel">
      <div className="crud-toolbar"><div><h2>Accepted documents</h2><p>{filtered.length} of {items.length} types</p></div><input className="crud-search" aria-label="Search identification types" placeholder="Search types…" value={query} onChange={(e) => setQuery(e.target.value)} /></div>
      {loading ? <div className="crud-state">Loading identification types…</div> : filtered.length === 0 ? <div className="crud-state"><strong>No identification types found</strong><span>Add a type or change your search.</span></div> : <div className="crud-table-wrap"><table className="crud-table"><thead><tr><th>Document</th><th>Code</th><th>Validation example</th><th>Status</th><th>Default</th><th className="crud-actions-heading">Actions</th></tr></thead><tbody>{filtered.map((item) => <tr key={item.id}><td><strong>{item.name}</strong><small className="crud-cell-subtext">{item.description || 'No description'}</small></td><td><code>{item.code}</code></td><td>{item.validation_example || '—'}</td><td><span className={`crud-pill ${item.is_active ? 'crud-pill-good' : 'crud-pill-muted'}`}>{item.is_active ? 'Active' : 'Inactive'}</span></td><td>{item.is_default ? 'Default' : '—'}</td><td><div className="crud-row-actions">{canManage && <button type="button" className="crud-button crud-button-muted" onClick={() => openEdit(item)}>Edit</button>}{canManage && <button type="button" className="crud-button crud-button-danger" disabled={item.is_default} onClick={() => void remove(item)}>Delete</button>}</div></td></tr>)}</tbody></table></div>}
    </div>
    {modal && <div className="crud-modal-backdrop" role="presentation" onMouseDown={(e) => { if (e.target === e.currentTarget && !busy) setModal(false); }}><section className="crud-modal" role="dialog" aria-modal="true" aria-labelledby="id-type-title"><header className="crud-modal-header"><div><div className="crud-eyebrow">IDENTIFICATION CATALOGUE</div><h2 id="id-type-title">{editing ? 'Edit identification type' : 'Add identification type'}</h2></div><button type="button" className="crud-close" disabled={busy} onClick={() => setModal(false)} aria-label="Close">×</button></header><form onSubmit={(e) => void save(e)}><div className="crud-form-grid">
      {!editing && <label className="crud-field">Code *<input required maxLength={50} value={form.code} onChange={(e) => setForm({ ...form, code: e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g,'') })} placeholder="e.g. NATIONAL_ID" /></label>}
      <label className="crud-field">Display name *<input required maxLength={100} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="National ID" /></label>
      <label className="crud-field crud-field-wide">Description<textarea rows={2} maxLength={1000} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} /></label>
      <label className="crud-field crud-field-wide">Validation regex (optional)<input value={form.validation_regex} onChange={(e) => setForm({ ...form, validation_regex: e.target.value })} placeholder="^[A-Z0-9-]{4,30}$" /></label>
      <label className="crud-field">Example<input value={form.validation_example} onChange={(e) => setForm({ ...form, validation_example: e.target.value })} placeholder="A12345678" /></label>
      <label className="crud-check-field"><input type="checkbox" checked={form.is_active} onChange={(e) => setForm({ ...form, is_active: e.target.checked })} /> Active and available for visitors</label>
      <label className="crud-check-field"><input type="checkbox" checked={form.is_default} onChange={(e) => setForm({ ...form, is_default: e.target.checked })} /> Use as default document type</label>
    </div>{formError && <div className="crud-inline-error">{formError}</div>}<footer className="crud-modal-footer"><button type="button" className="crud-button crud-button-muted" disabled={busy} onClick={() => setModal(false)}>Cancel</button><button type="submit" className="crud-button crud-button-primary" disabled={busy}>{busy ? 'Saving…' : 'Save identification type'}</button></footer></form></section></div>}
  </section>;
}
