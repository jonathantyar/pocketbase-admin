import { useMemo, useState } from 'react'
import { api, Field, Schema, errorText } from './api'
import Editor from './Editor'
import Drawer from './Drawer'

type Props = {
  project: string
  collection: string
  label?: string
  schema: Schema
  record: any | null
  actions: string[]
  onClose: () => void
  onSaved: () => void
}

export default function RecordForm({ project, collection, label, schema, record, actions, onClose, onSaved }: Props) {
  const isNew = !record
  const fields = useMemo(
    () => schema.fields.filter((f) => !f.system && f.name !== 'id' && f.type !== 'autodate' && f.type !== 'password'),
    [schema],
  )

  const [values, setValues] = useState<Record<string, any>>(() => {
    const v: Record<string, any> = {}
    for (const f of fields) v[f.name] = record?.[f.name] ?? blank(f)
    return v
  })
  const [files, setFiles] = useState<Record<string, File[]>>({})
  const [preview, setPreview] = useState<string | null>(null)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  const base = `/api/p/${project}/collections/${collection}/records`
  const canSave = isNew ? actions.includes('create') : actions.includes('update')

  function set(name: string, value: any) {
    setValues((prev) => ({ ...prev, [name]: value }))
  }

  async function save() {
    setBusy(true)
    setErr('')
    try {
      const payload: Record<string, any> = {}
      for (const f of fields) {
        if (f.type === 'file') continue
        const raw = values[f.name]
        if (raw === '' || raw === undefined || raw === null) continue
        if (f.type === 'number') payload[f.name] = Number(raw)
        else if (f.type === 'json') payload[f.name] = typeof raw === 'string' ? JSON.parse(raw) : raw
        else if (f.type === 'date') payload[f.name] = new Date(raw).toISOString()
        else payload[f.name] = raw
      }

      const withFiles = fields.some((f) => f.type === 'file' && (files[f.name]?.length ?? 0) > 0)
      if (withFiles) {
        const fd = new FormData()
        for (const [k, v] of Object.entries(payload)) {
          fd.append(k, typeof v === 'object' ? JSON.stringify(v) : String(v))
        }
        for (const f of fields) {
          if (f.type !== 'file') continue
          for (const file of files[f.name] ?? []) fd.append(f.name, file)
        }
        if (isNew) await api.form(base, fd, 'POST')
        else await api.form(`${base}/${record.id}`, fd, 'PATCH')
      } else if (isNew) {
        await api.post(base, payload)
      } else {
        await api.patch(`${base}/${record.id}`, payload)
      }
      onSaved()
    } catch (e) {
      setErr(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  async function remove() {
    if (!confirm('Delete this record?')) return
    setBusy(true)
    try {
      await api.del(`${base}/${record.id}`)
      onSaved()
    } catch (e) {
      setErr(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Drawer
      title={isNew ? `New ${label || collection}` : `Edit ${label || collection}`}
      onClose={onClose}
      footer={
        <>
          {!isNew && actions.includes('delete') && (
            <button className="btn danger ghost" onClick={remove} disabled={busy}>
              Delete
            </button>
          )}
          <div className="grow" />
          <button className="btn ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="btn" onClick={save} disabled={busy || !canSave}>
            {busy ? 'Saving…' : 'Save'}
          </button>
        </>
      }
    >
      {err && <div className="alert">{err}</div>}

      {fields.map((f) => (
        <div className="field" key={f.name}>
          <label>
            {f.name}
            {f.required && <span className="req"> *</span>}
            <span className="type">{f.type}</span>
          </label>
          <FieldInput
            field={f}
            value={values[f.name]}
            onChange={(v) => set(f.name, v)}
            recordId={record?.id}
            project={project}
            collection={collection}
            files={files[f.name] ?? []}
            onFiles={(list) => setFiles((prev) => ({ ...prev, [f.name]: list }))}
            onPreview={setPreview}
          />
        </div>
      ))}

      {preview && (
        <div className="modaloverlay" onClick={() => setPreview(null)}>
          <div className="modalbox" onClick={(e) => e.stopPropagation()}>
            <img src={preview} alt="" />
            <div className="modalfoot">
              <a className="btn ghost" href={preview} target="_blank" rel="noreferrer">
                Open original
              </a>
              <button className="btn" onClick={() => setPreview(null)}>
                Close
              </button>
            </div>
          </div>
        </div>
      )}
    </Drawer>
  )
}

function blank(f: Field) {
  if (f.type === 'bool') return false
  if (f.type === 'number') return ''
  if (f.type === 'select' && f.maxSelect > 1) return []
  if (f.type === 'json') return ''
  return ''
}

function FieldInput({
  field,
  value,
  onChange,
  recordId,
  project,
  collection,
  files,
  onFiles,
  onPreview,
}: {
  field: Field
  value: any
  onChange: (v: any) => void
  recordId?: string
  project: string
  collection: string
  files: File[]
  onFiles: (f: File[]) => void
  onPreview: (url: string) => void
}) {
  const id = `f-${field.name}`

  switch (field.type) {
    case 'bool':
      return (
        <label className="check">
          <input id={id} type="checkbox" checked={!!value} onChange={(e) => onChange(e.target.checked)} />
          <span className="muted small">{value ? 'yes' : 'no'}</span>
        </label>
      )

    case 'number':
      return <input id={id} type="number" value={value ?? ''} onChange={(e) => onChange(e.target.value)} />

    case 'email':
      return <input id={id} type="email" value={value ?? ''} onChange={(e) => onChange(e.target.value)} />

    case 'url':
      return <input id={id} type="url" value={value ?? ''} onChange={(e) => onChange(e.target.value)} />

    case 'date':
      return (
        <input
          id={id}
          type="datetime-local"
          value={toLocal(value)}
          onChange={(e) => onChange(e.target.value)}
        />
      )

    case 'select': {
      const opts = field.values ?? []
      if (field.maxSelect > 1) {
        const selected: string[] = Array.isArray(value) ? value : []
        return (
          <div className="checks">
            {opts.map((o) => (
              <label className="check" key={o}>
                <input
                  type="checkbox"
                  checked={selected.includes(o)}
                  onChange={(e) =>
                    onChange(e.target.checked ? [...selected, o] : selected.filter((x) => x !== o))
                  }
                />
                {o}
              </label>
            ))}
          </div>
        )
      }
      return (
        <select id={id} value={value ?? ''} onChange={(e) => onChange(e.target.value)}>
          <option value="">—</option>
          {opts.map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </select>
      )
    }

    case 'editor':
      return <Editor value={value ?? ''} onChange={onChange} />

    case 'json':
      return (
        <textarea
          id={id}
          rows={5}
          className="mono"
          value={typeof value === 'string' ? value : JSON.stringify(value ?? {}, null, 2)}
          onChange={(e) => onChange(e.target.value)}
        />
      )

    case 'relation':
      // shortcut: relation is edited as a raw record id; turn into a picker when
      // the schema endpoint exposes the target collection name.
      return (
        <input
          id={id}
          value={Array.isArray(value) ? value.join(',') : (value ?? '')}
          placeholder="record id (comma separated for multiple)"
          onChange={(e) => {
            const raw = e.target.value.trim()
            if (field.maxSelect > 1) onChange(raw ? raw.split(',').map((s) => s.trim()) : [])
            else onChange(raw)
          }}
        />
      )

    case 'file': {
      const existing: string[] = Array.isArray(value) ? value : value ? [value] : []
      return (
        <div className="files">
          {existing.length > 0 && (
            <div className="existing">
              {existing.map((name) => {
                const url = `/api/p/${project}/files/${collection}/${recordId}/${name}`
                const label = name.length > 28 ? name.slice(0, 28) + '…' : name
                return isImage(name) ? (
                  <button key={name} type="button" className="chip" onClick={() => onPreview(url)}>
                    {label}
                  </button>
                ) : (
                  <a key={name} href={url} target="_blank" rel="noreferrer" className="chip">
                    {label}
                  </a>
                )
              })}
            </div>
          )}
          <input
            id={id}
            type="file"
            multiple={field.maxSelect !== 1}
            onChange={(e) => onFiles(Array.from(e.target.files ?? []))}
          />
          {files.length > 0 && (
            <p className="muted small">
              {files.length} new file{files.length > 1 ? 's' : ''} selected
            </p>
          )}
        </div>
      )
    }

    default:
      return <input id={id} value={value ?? ''} onChange={(e) => onChange(e.target.value)} />
  }
}

function isImage(name: string) {
  return /\.(png|jpe?g|gif|webp|avif|svg|bmp)$/i.test(name)
}

function toLocal(v: any): string {
  if (!v) return ''
  const d = new Date(String(v).replace(' ', 'T'))
  if (isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
