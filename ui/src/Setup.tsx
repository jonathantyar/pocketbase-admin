import { useEffect, useMemo, useState } from 'react'
import { api, errorText } from './api'
import Drawer from './Drawer'
import Icon, { ICON_NAMES } from './Icon'
import ProjectPicker from './ProjectPicker'

const ACTIONS = ['list', 'view', 'create', 'update', 'delete']
const DEFAULT_ROLES: Record<string, string[]> = {
  owner: ['*'],
  editor: ['list', 'view', 'create', 'update'],
  viewer: ['list', 'view'],
}

type Modal = null | 'project' | 'collection' | 'user'

export default function Setup({
  project,
  email,
  onSaved,
}: {
  project: string
  email: string
  onSaved: () => void
}) {
  const [cfg, setCfg] = useState<any>(null)
  const [pbCols, setPbCols] = useState<any[]>([])
  const [active, setActive] = useState(project)
  const [modal, setModal] = useState<Modal>(null)
  const [roles, setRoles] = useState<Record<string, string[]>>(DEFAULT_ROLES)
  const [app, setApp] = useState({ appName: '', appDescription: '', logo: 'box', pocketbaseVersion: '' })
  const [err, setErr] = useState('')
  const [msg, setMsg] = useState('')

  async function loadCfg() {
    try {
      setCfg(await api.get('/api/setup/config'))
    } catch (e) {
      setErr(errorText(e))
    }
  }

  useEffect(() => {
    void loadCfg()
    api
      .get('/api/setup/collections')
      .then((r: any) => setPbCols(r.items ?? []))
      .catch((e) => setErr(errorText(e)))
  }, [])

  useEffect(() => {
    setActive(project)
  }, [project])

  const p = cfg?.projects?.[active]
  const names = Object.keys(cfg?.projects ?? {})
  const roleKeys = Object.keys(roles).sort((a, b) => (a === 'owner' ? -1 : b === 'owner' ? 1 : a.localeCompare(b)))

  useEffect(() => {
    if (!p) return
    const r: Record<string, string[]> = {}
    for (const [k, v] of Object.entries<any>(p.roles ?? {})) r[k] = v.actions ?? []
    setRoles(Object.keys(r).length ? r : DEFAULT_ROLES)
  }, [cfg, active, p])

  useEffect(() => {
    if (!cfg?.server) return
    setApp({
      appName: cfg.server.appName ?? '',
      appDescription: cfg.server.appDescription ?? '',
      logo: cfg.server.logo ?? 'box',
      pocketbaseVersion: cfg.server.pocketbaseVersion ?? '',
    })
  }, [cfg])

  async function saveApp() {
    setErr('')
    setMsg('')
    if (!app.appName.trim()) return setErr('App name is required.')
    try {
      await api.post('/api/setup/app', {
        appName: app.appName.trim(),
        appDescription: app.appDescription.trim(),
        logo: app.logo.trim(),
        pocketbaseVersion: app.pocketbaseVersion.trim(),
      })
      await done('Branding saved.')
    } catch (e) {
      setErr(errorText(e))
    }
  }

  async function done(message?: string) {
    setModal(null)
    setMsg(message ?? 'Saved.')
    setErr('')
    await loadCfg()
    onSaved()
  }

  async function saveRoles() {
    setErr('')
    setMsg('')
    const out: Record<string, any> = {}
    for (const [k, v] of Object.entries(roles)) {
      out[k] = { label: k.charAt(0).toUpperCase() + k.slice(1), actions: v }
    }
    try {
      await api.post('/api/setup/project', { name: active, roles: out })
      await done('Roles saved.')
    } catch (e) {
      setErr(errorText(e))
    }
  }

  async function removeCollection(name: string) {
    if (!confirm(`Remove "${name}" from this project's allowlist?`)) return
    try {
      await api.del(`/api/setup/collection?project=${encodeURIComponent(active)}&name=${encodeURIComponent(name)}`)
      await done('Collection removed.')
    } catch (e) {
      setErr(errorText(e))
    }
  }

  return (
    <div>
      <div className="pagehead">
        <div>
          <h1>Settings</h1>
          <p className="muted small">
            PocketBase: <code>{cfg?.pocketbase?.url ?? '…'}</code>
          </p>
        </div>
        <button className="btn" onClick={() => setModal('project')}>
          New project
        </button>
      </div>

      {err && <div className="alert">{err}</div>}
      {msg && <div className="okbox">{msg}</div>}

      {names.length > 0 && (
        <div className="field narrow">
          <label>Project</label>
          <ProjectPicker
            projects={names.map((n) => ({
              name: n,
              label: cfg.projects[n].label || n,
              icon: cfg.projects[n].icon,
            }))}
            value={active}
            onChange={setActive}
          />
        </div>
      )}

      {!p && <div className="empty">No projects yet. Create one to get started.</div>}

      <section className="card">
        <div className="cardhead">
          <h2>Appearance</h2>
          <button className="btn ghost" onClick={saveApp}>
            Save branding
          </button>
        </div>
        <div className="pad">
          <div className="cols2">
            <div className="field">
              <label>
                App name <span className="type">browser title</span>
              </label>
              <input
                value={app.appName}
                onChange={(e) => setApp({ ...app, appName: e.target.value })}
                placeholder="pocketbase-admin"
              />
            </div>
            <div className="field">
              <label>
                PocketBase version <span className="type">optional</span>
              </label>
              <input
                value={app.pocketbaseVersion}
                onChange={(e) => setApp({ ...app, pocketbaseVersion: e.target.value })}
                placeholder="0.40.5"
              />
            </div>
          </div>
          <div className="field">
            <label>
              Description <span className="type">meta description</span>
            </label>
            <input
              value={app.appDescription}
              onChange={(e) => setApp({ ...app, appDescription: e.target.value })}
              placeholder="Admin panel for PocketBase with multi-tenant RBAC"
            />
          </div>
          <div className="field">
            <label>Logo icon</label>
            <div className="iconpick">
              {ICON_NAMES.map((i) => (
                <button
                  type="button"
                  key={i}
                  className={app.logo === i ? 'on' : ''}
                  onClick={() => setApp({ ...app, logo: i })}
                  title={i}
                >
                  <Icon name={i} size={17} />
                </button>
              ))}
            </div>
          </div>
        </div>
      </section>

      {p && (
        <>
          <section className="card">
            <div className="cardhead">
              <h2>Collections</h2>
              <button className="btn ghost" onClick={() => setModal('collection')}>
                Add collection
              </button>
            </div>
            <table>
              <thead>
                <tr>
                  <th className="iconcol">Icon</th>
                  <th>Name</th>
                  <th>Collection</th>
                  <th className="actioncol" />
                </tr>
              </thead>
              <tbody>
                {Object.entries<any>(p.collections ?? {}).map(([name, meta]) => (
                  <tr key={name}>
                    <td className="iconcol">
                      <Icon name={meta.icon || 'file'} size={17} />
                    </td>
                    <td>{meta.label || name}</td>
                    <td className="muted mono tiny">{name}</td>
                    <td className="actioncol">
                      <button className="btn ghost icon" onClick={() => removeCollection(name)} title="Remove">
                        ✕
                      </button>
                    </td>
                  </tr>
                ))}
                {Object.keys(p.collections ?? {}).length === 0 && (
                  <tr>
                    <td colSpan={4} className="muted small">
                      No collections yet. Add one from the live PocketBase list.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </section>

          <section className="card">
            <div className="cardhead">
              <h2>Roles</h2>
              <button className="btn ghost" onClick={saveRoles}>
                Save roles
              </button>
            </div>
            <table className="matrix">
              <thead>
                <tr>
                  <th>Role</th>
                  {ACTIONS.map((a) => (
                    <th key={a}>{a}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {roleKeys.map((r) => (
                  <tr key={r}>
                    <td>{r}</td>
                    {ACTIONS.map((a) => {
                      const on = r === 'owner' || (roles[r] ?? []).includes(a)
                      return (
                        <td key={a}>
                          <input
                            type="checkbox"
                            checked={on}
                            disabled={r === 'owner'}
                            onChange={() => toggleAction(r, a, roles, setRoles)}
                          />
                        </td>
                      )
                    })}
                  </tr>
                ))}
              </tbody>
            </table>
          </section>

          <section className="card">
            <div className="cardhead">
              <h2>Users</h2>
              <button className="btn ghost" onClick={() => setModal('user')}>
                Add user
              </button>
            </div>
            <table>
              <thead>
                <tr>
                  <th>Email</th>
                  <th>Role</th>
                </tr>
              </thead>
              <tbody>
                {(p.users ?? []).map((u: any) => (
                  <tr key={u.email}>
                    <td>{u.email}</td>
                    <td className="muted">{u.role}</td>
                  </tr>
                ))}
                {(p.users ?? []).length === 0 && (
                  <tr>
                    <td colSpan={2} className="muted small">
                      Nobody can sign in to this project yet.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </section>
        </>
      )}

      {modal === 'project' && (
        <NewProjectDrawer
          email={email}
          onClose={() => setModal(null)}
          onDone={(name) => {
            setActive(name)
            void done('Project created.')
          }}
        />
      )}
      {modal === 'collection' && p && (
        <AddCollectionDrawer
          project={active}
          pbCols={pbCols}
          selected={p.collections ?? {}}
          onClose={() => setModal(null)}
          onDone={() => void done('Collection added.')}
        />
      )}
      {modal === 'user' && p && (
        <AddUserDrawer
          project={active}
          roles={Object.keys(roles)}
          onClose={() => setModal(null)}
          onDone={() => void done('User added.')}
        />
      )}
    </div>
  )
}

function toggleAction(
  role: string,
  action: string,
  roles: Record<string, string[]>,
  setRoles: (r: Record<string, string[]>) => void,
) {
  if (role === 'owner') return
  const cur = new Set(roles[role] ?? [])
  cur.has(action) ? cur.delete(action) : cur.add(action)
  setRoles({ ...roles, [role]: ACTIONS.filter((a) => cur.has(a)) })
}

function NewProjectDrawer({
  email,
  onClose,
  onDone,
}: {
  email: string
  onClose: () => void
  onDone: (name: string) => void
}) {
  const [name, setName] = useState('')
  const [label, setLabel] = useState('')
  const [icon, setIcon] = useState('folder')
  const [owner, setOwner] = useState(email)
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit() {
    setErr('')
    if (!name.trim()) return setErr('Project name is required.')
    if (!owner.trim()) return setErr('An owner email is required.')
    if (password.length < 8) return setErr('Owner password must be at least 8 characters.')
    setBusy(true)
    try {
      await api.post('/api/setup/project', {
        name: name.trim(),
        label: label.trim() || name.trim(),
        icon,
        roles: {
          owner: { label: 'Owner', actions: ['*'] },
          editor: { label: 'Editor', actions: ['list', 'view', 'create', 'update'] },
          viewer: { label: 'Viewer', actions: ['list', 'view'] },
        },
      })
      await api.post('/api/setup/user', {
        project: name.trim(),
        email: owner.trim(),
        password,
        role: 'owner',
      })
      onDone(name.trim())
    } catch (e) {
      setErr(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Drawer
      title="New project"
      onClose={onClose}
      footer={
        <>
          <div className="grow" />
          <button className="btn ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="btn" onClick={submit} disabled={busy}>
            {busy ? 'Creating…' : 'Create project'}
          </button>
        </>
      }
    >
      {err && <div className="alert">{err}</div>}
      <div className="field">
        <label>
          Name <span className="type">used in URLs</span>
        </label>
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder="myproject" />
      </div>
      <div className="field">
        <label>Label</label>
        <input value={label} onChange={(e) => setLabel(e.target.value)} placeholder="My Project" />
      </div>
      <div className="field">
        <label>Icon</label>
        <div className="iconpick">
          {ICON_NAMES.map((i) => (
            <button type="button" key={i} className={icon === i ? 'on' : ''} onClick={() => setIcon(i)} title={i}>
              <Icon name={i} size={17} />
            </button>
          ))}
        </div>
      </div>
      <p className="muted small">A project needs an owner account. You can add more users later.</p>
      <div className="field">
        <label>Owner email</label>
        <input value={owner} onChange={(e) => setOwner(e.target.value)} />
      </div>
      <div className="field">
        <label>
          Owner password <span className="type">min 8 chars</span>
        </label>
        <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
      </div>
    </Drawer>
  )
}

function AddCollectionDrawer({
  project,
  pbCols,
  selected,
  onClose,
  onDone,
}: {
  project: string
  pbCols: any[]
  selected: Record<string, any>
  onClose: () => void
  onDone: () => void
}) {
  const [search, setSearch] = useState('')
  const [picked, setPicked] = useState('')
  const [label, setLabel] = useState('')
  const [icon, setIcon] = useState('file')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  const visible = useMemo(() => {
    const q = search.trim().toLowerCase()
    return pbCols.filter((c) => !c.system && !selected[c.name] && (!q || c.name.toLowerCase().includes(q)))
  }, [pbCols, search, selected])

  function pick(name: string) {
    setPicked(name)
    setLabel(name)
  }

  async function submit() {
    if (!picked) return setErr('Pick a collection first.')
    setBusy(true)
    setErr('')
    try {
      await api.post('/api/setup/collection', { project, name: picked, label: label.trim() || picked, icon })
      onDone()
    } catch (e) {
      setErr(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Drawer
      wide
      title="Add collection"
      onClose={onClose}
      footer={
        <>
          <div className="grow" />
          <button className="btn ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="btn" onClick={submit} disabled={busy || !picked}>
            {busy ? 'Adding…' : 'Add collection'}
          </button>
        </>
      }
    >
      {err && <div className="alert">{err}</div>}

      <div className="field">
        <label>Search collections</label>
        <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search…" />
      </div>
      <div className="collist">
        {visible.map((c) => (
          <label className={`check ${picked === c.name ? 'picked' : ''}`} key={c.name}>
            <input type="radio" name="collection" checked={picked === c.name} onChange={() => pick(c.name)} />
            <span>{c.name}</span>
            <span className="muted tiny">{c.type}</span>
          </label>
        ))}
        {visible.length === 0 && <p className="muted small">No matching collections.</p>}
      </div>

      <div className="field">
        <label>Display name</label>
        <input value={label} onChange={(e) => setLabel(e.target.value)} placeholder="Posts" />
      </div>
      <div className="field">
        <label>Icon</label>
        <div className="iconpick">
          {ICON_NAMES.map((i) => (
            <button type="button" key={i} className={icon === i ? 'on' : ''} onClick={() => setIcon(i)} title={i}>
              <Icon name={i} size={17} />
            </button>
          ))}
        </div>
        <input value={icon} onChange={(e) => setIcon(e.target.value)} placeholder="icon name or emoji" />
      </div>
    </Drawer>
  )
}

function AddUserDrawer({
  project,
  roles,
  onClose,
  onDone,
}: {
  project: string
  roles: string[]
  onClose: () => void
  onDone: () => void
}) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState(roles[1] ?? roles[0] ?? 'owner')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit() {
    setErr('')
    if (!email.trim()) return setErr('Email is required.')
    if (password.length < 8) return setErr('Password must be at least 8 characters.')
    setBusy(true)
    try {
      await api.post('/api/setup/user', { project, email: email.trim(), password, role })
      onDone()
    } catch (e) {
      setErr(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Drawer
      title="Add user"
      onClose={onClose}
      footer={
        <>
          <div className="grow" />
          <button className="btn ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="btn" onClick={submit} disabled={busy}>
            {busy ? 'Saving…' : 'Add user'}
          </button>
        </>
      }
    >
      {err && <div className="alert">{err}</div>}
      <div className="field">
        <label>Email</label>
        <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="off" />
      </div>
      <div className="field">
        <label>
          Password <span className="type">min 8 chars</span>
        </label>
        <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
      </div>
      <div className="field">
        <label>Role</label>
        <select value={role} onChange={(e) => setRole(e.target.value)}>
          {roles.map((r) => (
            <option key={r} value={r}>
              {r}
            </option>
          ))}
        </select>
      </div>
      <p className="muted small">
        Adding an email that already exists in this project resets its password and role.
      </p>
    </Drawer>
  )
}
