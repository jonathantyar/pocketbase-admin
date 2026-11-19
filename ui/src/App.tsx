import { useCallback, useEffect, useState } from 'react'
import { api, Me, Meta } from './api'
import Login from './Login'
import Data from './Data'
import Setup from './Setup'
import Icon, { GithubIcon } from './Icon'
import ProjectPicker from './ProjectPicker'

type Boot = 'loading' | 'login' | 'bootstrap' | 'ready'

const FAVICON =
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="#2f7fe0"/><circle cx="16" cy="16" r="5.2" fill="#fff"/></svg>'

function applyBranding(m: Meta) {
  document.title = m.appName

  let desc = document.querySelector('meta[name="description"]')
  if (!desc) {
    desc = document.createElement('meta')
    desc.setAttribute('name', 'description')
    document.head.appendChild(desc)
  }
  desc.setAttribute('content', m.appDescription)

  let link = document.querySelector("link[rel='icon']") as HTMLLinkElement | null
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }
  link.href = 'data:image/svg+xml,' + encodeURIComponent(FAVICON)
}

export default function App() {
  const [boot, setBoot] = useState<Boot>('loading')
  const [me, setMe] = useState<Me | null>(null)
  const [meta, setMeta] = useState<Meta | null>(null)
  const [project, setProject] = useState('')
  const [cols, setCols] = useState<{ name: string; label: string; icon?: string; listFields?: string[] }[]>([])
  const [tab, setTab] = useState('')
  const [page, setPage] = useState<'data' | 'setup'>('data')

  const refreshMeta = useCallback(async () => {
    try {
      const m = (await api.get('/api/meta')) as Meta
      setMeta(m)
      applyBranding(m)
    } catch {
      // Branding is cosmetic; ignore failures.
    }
  }, [])

  const load = useCallback(async () => {
    try {
      const m = (await api.get('/api/me')) as Me
      setMe(m)
      setProject((prev) => prev || m.projects[0]?.name || '')
      setBoot('ready')
    } catch (e: any) {
      if (e?.status === 401) {
        // No users configured yet ⇒ the bootstrap endpoint answers without auth.
        try {
          await api.get('/api/setup/config')
          setBoot('bootstrap')
        } catch {
          setBoot('login')
        }
      } else {
        setBoot('login')
      }
    }
  }, [])

  const loadCols = useCallback(async (p: string) => {
    if (!p) return
    try {
      const r: any = await api.get(`/api/p/${p}/collections`)
      const list = r.collections ?? []
      setCols(list)
      setTab((cur) => (list.some((c: any) => c.name === cur) ? cur : (list[0]?.name ?? '')))
    } catch {
      setCols([])
    }
  }, [])

  useEffect(() => {
    void refreshMeta()
    void load()
  }, [refreshMeta, load])

  useEffect(() => {
    if (boot === 'ready' && project) void loadCols(project)
  }, [boot, project, loadCols])

  async function logout() {
    await api.post('/api/logout')
    setMe(null)
    setProject('')
    setBoot('login')
  }

  if (boot === 'loading') return <div className="center muted">Loading…</div>
  if (boot === 'login' || boot === 'bootstrap') {
    return <Login bootstrap={boot === 'bootstrap'} onDone={load} meta={meta} />
  }

  const projects = me?.projects ?? []
  const perms = me?.projects.find((p) => p.name === project)?.collections.find((c) => c.name === tab)?.actions ?? []
  const meta2 = cols.find((c) => c.name === tab)

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand">
          <span className="brandmark">
            <Icon name={meta?.logo || 'box'} size={16} />
          </span>
          <span className="brandname">{meta?.appName || 'pocketbase-admin'}</span>
          {meta?.pocketbaseVersion && <span className="badge">PocketBase {meta.pocketbaseVersion}</span>}
        </div>
        <div className="grow" />
        {meta?.repository && (
          <a
            className="iconlink"
            href={meta.repository}
            target="_blank"
            rel="noreferrer"
            title="GitHub repository"
            aria-label="GitHub repository"
          >
            <GithubIcon />
          </a>
        )}
        <span className="muted small">{me?.email}</span>
        <button className="btn ghost" onClick={logout}>
          Sign out
        </button>
      </header>

      <div className="layout">
        <aside className="sidebar">
          {projects.length > 1 && (
            <div className="field">
              <label>Project</label>
              <ProjectPicker
                projects={projects}
                value={project}
                onChange={(name) => {
                  setProject(name)
                  setPage('data')
                }}
              />
            </div>
          )}

          <nav>
            {cols.map((c) => (
              <button
                key={c.name}
                className={`navitem ${tab === c.name && page === 'data' ? 'active' : ''}`}
                onClick={() => {
                  setTab(c.name)
                  setPage('data')
                }}
              >
                <span className="ic">
                  <Icon name={c.icon || 'file'} />
                </span>
                <span className="navlabel">{c.label || c.name}</span>
              </button>
            ))}
            {cols.length === 0 && <p className="muted small pad">No collections in this project yet.</p>}
          </nav>

          <div className="grow" />
          {me?.setup && (
            <button className={`navitem ${page === 'setup' ? 'active' : ''}`} onClick={() => setPage('setup')}>
              <span className="ic">
                <Icon name="settings" />
              </span>
              <span className="navlabel">Settings</span>
            </button>
          )}
        </aside>

        <main className="content">
          {page === 'setup' && me?.setup ? (
            <Setup
              project={project}
              email={me?.email ?? ''}
              onSaved={() => {
                void refreshMeta()
                void load()
                void loadCols(project)
              }}
            />
          ) : meta2 ? (
            <Data project={project} meta={meta2} actions={perms} />
          ) : (
            <div className="empty">
              <h2>Nothing to show</h2>
              <p className="muted">This project has no collections, or your role cannot list any.</p>
            </div>
          )}
        </main>
      </div>
    </div>
  )
}
