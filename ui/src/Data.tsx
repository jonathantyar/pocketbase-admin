import { useEffect, useMemo, useState } from 'react'
import { api, CollectionMeta, Field, Schema, errorText } from './api'
import RecordForm from './RecordForm'

export default function Data({
  project,
  meta,
  actions,
}: {
  project: string
  meta: CollectionMeta
  actions: string[]
}) {
  const [schema, setSchema] = useState<Schema | null>(null)
  const [rows, setRows] = useState<any[]>([])
  const [page, setPage] = useState(1)
  const [pages, setPages] = useState(1)
  const [total, setTotal] = useState(0)
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)
  const [editing, setEditing] = useState<any | 'new' | null>(null)

  const base = `/api/p/${project}/collections/${meta.name}`

  async function loadRows(p = 1) {
    setLoading(true)
    setErr('')
    try {
      const r: any = await api.get(`${base}/records?perPage=50&page=${p}`)
      setRows(r.items ?? [])
      setPages(r.totalPages ?? 1)
      setTotal(r.totalItems ?? 0)
      setPage(p)
    } catch (e) {
      setErr(errorText(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadRows(1)
    setSchema(null)
    api
      .get(`/api/p/${project}/schema/${meta.name}`)
      .then((s: any) => setSchema(s))
      .catch((e) => setErr(errorText(e)))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [base])

  const cols = useMemo<Field[]>(() => {
    if (!schema) return []
    const fields = schema.fields.filter((f) => !f.system && f.name !== 'id' && f.type !== 'autodate')
    const wanted = meta.listFields?.length ? meta.listFields : fields.slice(0, 5).map((f) => f.name)
    return wanted.map((n) => fields.find((f) => f.name === n)).filter((f): f is Field => !!f)
  }, [schema, meta])

  return (
    <div>
      <div className="pagehead">
        <div>
          <h1>{meta.label || meta.name}</h1>
          <p className="muted small">
            {meta.name} · {total} {total === 1 ? 'record' : 'records'}
          </p>
        </div>
        {actions.includes('create') && (
          <button className="btn" onClick={() => setEditing('new')}>
            New record
          </button>
        )}
      </div>

      {err && <div className="alert">{err}</div>}

      <div className="card tablewrap">
        <table>
          <thead>
            <tr>
              {cols.map((c) => (
                <th key={c.name}>{c.name}</th>
              ))}
              <th className="idcol" />
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.id} onClick={() => setEditing(r)}>
                {cols.map((c) => (
                  <td key={c.name}>{cell(r[c.name], c)}</td>
                ))}
                <td className="idcol muted tiny">{String(r.id).slice(0, 8)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {!loading && rows.length === 0 && <div className="empty small">No records yet.</div>}
        {loading && <div className="empty small muted">Loading…</div>}
      </div>

      <div className="pager">
        <button className="btn ghost" disabled={page <= 1} onClick={() => loadRows(page - 1)}>
          Previous
        </button>
        <span className="muted small">
          Page {page} of {pages}
        </span>
        <button className="btn ghost" disabled={page >= pages} onClick={() => loadRows(page + 1)}>
          Next
        </button>
      </div>

      {editing !== null && schema && (
        <RecordForm
          project={project}
          collection={meta.name}
          label={meta.label}
          schema={schema}
          record={editing === 'new' ? null : editing}
          actions={actions}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null)
            void loadRows(page)
          }}
        />
      )}
    </div>
  )
}

function cell(v: any, f: Field) {
  if (v === null || v === undefined || v === '') return <span className="muted">—</span>
  if (f.type === 'bool') return v ? '✓' : '—'
  if (f.type === 'date') {
    const d = new Date(String(v).replace(' ', 'T'))
    if (!isNaN(d.getTime())) return d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
  }
  if (Array.isArray(v)) return trunc(v.join(', '))
  if (typeof v === 'object') return trunc(JSON.stringify(v))
  if (f.type === 'editor') return trunc(String(v).replace(/<[^>]+>/g, ' ').trim())
  return trunc(String(v))
}

function trunc(s: string) {
  return s.length > 64 ? s.slice(0, 64) + '…' : s
}
