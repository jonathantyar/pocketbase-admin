export type ActionName = 'list' | 'view' | 'create' | 'update' | 'delete'

export type Meta = {
  appName: string
  appDescription: string
  logo: string
  pocketbaseVersion: string
  repository: string
  version: string
}

export type Me = {
  email: string
  setup: boolean
  projects: {
    name: string
    label: string
    icon?: string
    role: string
    collections: { name: string; actions: string[] }[]
  }[]
}

export type CollectionMeta = {
  name: string
  label: string
  icon?: string
  listFields?: string[]
}

export type Field = {
  name: string
  type: string
  required: boolean
  maxSelect: number
  system: boolean
  values?: string[] | null
  help?: string
  options?: Record<string, unknown>
  collectionId?: string
}

export type Schema = {
  name: string
  fields: Field[]
}

export class ApiError extends Error {
  status: number
  body: any
  constructor(message: string, status: number, body: any) {
    super(message)
    this.status = status
    this.body = body
  }
}

async function request(path: string, init?: RequestInit) {
  const res = await fetch(path, { credentials: 'same-origin', ...init })
  const text = await res.text()
  let data: any = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = { error: text }
  }
  if (!res.ok) {
    const msg = data?.error || data?.message || `HTTP ${res.status}`
    throw new ApiError(msg, res.status, data)
  }
  return data
}

const json = (method: string) => (path: string, body?: any) =>
  request(path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body ?? {}),
  })

export const api = {
  get: (path: string) => request(path),
  post: json('POST'),
  patch: json('PATCH'),
  del: (path: string) => request(path, { method: 'DELETE' }),
  form: (path: string, form: FormData, method = 'POST') => request(path, { method, body: form }),
}

// PocketBase returns per-field validation errors in body.data.
export function errorText(e: any): string {
  const data = e?.body?.data
  if (data && typeof data === 'object') {
    const parts = Object.entries(data).map(([k, v]: [string, any]) => `${k}: ${v?.message ?? 'tidak valid'}`)
    if (parts.length) return parts.join(' · ')
  }
  return e?.message ?? 'Terjadi kesalahan'
}
