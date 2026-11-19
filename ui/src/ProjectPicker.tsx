import { useState } from 'react'
import Icon from './Icon'

export type PickerProject = { name: string; label: string; icon?: string }

export default function ProjectPicker({
  projects,
  value,
  onChange,
}: {
  projects: PickerProject[]
  value: string
  onChange: (name: string) => void
}) {
  const [open, setOpen] = useState(false)
  const current = projects.find((p) => p.name === value)

  return (
    <div className="picker">
      <button type="button" className="pickerbtn" onClick={() => setOpen((o) => !o)}>
        <Icon name={current?.icon || 'folder'} />
        <span className="navlabel">{current?.label || current?.name || 'Select project'}</span>
        <svg
          className="chev"
          width="13"
          height="13"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="m6 9 6 6 6-6" />
        </svg>
      </button>

      {open && (
        <>
          <div className="pickerbackdrop" onClick={() => setOpen(false)} />
          <div className="pickermenu">
            {projects.map((p) => (
              <button
                type="button"
                key={p.name}
                className={p.name === value ? 'picked' : ''}
                onClick={() => {
                  onChange(p.name)
                  setOpen(false)
                }}
              >
                <Icon name={p.icon || 'folder'} />
                <span className="navlabel">{p.label || p.name}</span>
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  )
}
