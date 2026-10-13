import { useEffect, useState } from 'react';
import type { CrudRecord } from './types';
type Props = { columns: {key:string;label:string;format?:(row:CrudRecord)=>string}[]; rows: CrudRecord[]; loading: boolean; error?: string; onEdit?: (row:CrudRecord)=>void; onDelete?: (row:CrudRecord)=>void; canEdit: boolean; canDelete: boolean; deleteLabel?: string; empty: string };
const PAGE_SIZE=10;
export default function DataTable({columns,rows,loading,error,onEdit,onDelete,canEdit,canDelete,deleteLabel="Delete",empty}:Props) {
 const [page,setPage]=useState(1);useEffect(()=>setPage(1),[rows.length]);
 const pages=Math.max(1,Math.ceil(rows.length/PAGE_SIZE));const visible=rows.slice((page-1)*PAGE_SIZE,page*PAGE_SIZE);
 if (loading) return <div className="crud-state"><span className="tenant-spinner"/> Loading records…</div>;
 if (error) return <div className="crud-state crud-state-error" role="alert">{error}</div>;
 if (!rows.length) return <div className="crud-state"><strong>No records found</strong><span>{empty}</span></div>;
 return <><div className="crud-table-wrap"><table className="crud-table"><thead><tr>{columns.map(c=><th key={c.key}>{c.label}</th>)}{(canEdit||canDelete)&&<th className="crud-actions-heading">Actions</th>}</tr></thead><tbody>{visible.map(row=><tr key={row.id}>{columns.map(c=><td key={c.key}>{c.format?c.format(row):String(row[c.key]??'—')}</td>)}{(canEdit||canDelete)&&<td><div className="crud-row-actions">{canEdit&&onEdit&&<button type="button" className="crud-button crud-button-muted" onClick={()=>onEdit(row)}>Edit</button>}{canDelete&&onDelete&&<button type="button" className="crud-button crud-button-danger" onClick={()=>onDelete(row)}>{deleteLabel}</button>}</div></td>}</tr>)}</tbody></table></div><footer className="crud-pagination"><span>Showing {(page-1)*PAGE_SIZE+1}–{Math.min(page*PAGE_SIZE,rows.length)} of {rows.length}</span><div><button type="button" className="crud-button crud-button-muted" disabled={page<=1} onClick={()=>setPage(v=>Math.max(1,v-1))}>Previous</button><span>Page {page} of {pages}</span><button type="button" className="crud-button crud-button-muted" disabled={page>=pages} onClick={()=>setPage(v=>Math.min(pages,v+1))}>Next</button></div></footer></>;
}
