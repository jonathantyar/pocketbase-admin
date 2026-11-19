import { useState } from 'react'
import { api, errorText, Meta } from './api'
import Icon from './Icon'

export default function Login({
  bootstrap,
  onDone,
  meta,
}: {
  bootstrap: boolean
  onDone: () => void
  meta: Meta | null
}) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [project, setProject] = useState('default')
  const [label, setLabel] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setErr('')
    try {
      if (bootstrap) {
        await api.post('/api/setup/bootstrap', {
          project: project.trim(),
          label: label.trim(),
          email: email.trim(),
          password,
        })
      } else {
        await api.post('/api/login', { email: email.trim(), password })
      }
      onDone()
    } catch (e) {
      setErr(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="auth">
      <div className="authwrap">
        <div className="authbrand">
          <span className="brandmark">
            <Icon name={meta?.logo || 'box'} size={16} />
          </span>
          <span className="brandname">{meta?.appName || 'pocketbase-admin'}</span>
        </div>
        <form className="card authcard" onSubmit={submit}>
        <h1>{bootstrap ? 'First-time setup' : 'Sign in'}</h1>
        <p className="muted small">
          {bootstrap
            ? 'No users exist yet. Create the first owner account to get started.'
            : 'Sign in with an account created in pocketbase-admin, not a PocketBase account.'}
        </p>

        {bootstrap && (
          <>
            <div className="field">
              <label>Project name</label>
              <input value={project} onChange={(e) => setProject(e.target.value)} placeholder="myproject" required />
            </div>
            <div className="field">
              <label>Project label</label>
              <input value={label} onChange={(e) => setLabel(e.target.value)} placeholder="My Project" />
            </div>
          </>
        )}

        <div className="field">
          <label>Email</label>
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="username"
            required
          />
        </div>
        <div className="field">
          <label>Password</label>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete={bootstrap ? 'new-password' : 'current-password'}
            minLength={bootstrap ? 8 : undefined}
            required
          />
        </div>

        {err && <div className="alert">{err}</div>}

        <button className="btn full" disabled={busy}>
          {busy ? 'Working…' : bootstrap ? 'Create owner' : 'Sign in'}
        </button>
        </form>
      </div>
    </div>
  )
}
