import { ReactNode } from 'react'

export default function Drawer({
  title,
  onClose,
  footer,
  children,
  wide,
}: {
  title: ReactNode
  onClose: () => void
  footer?: ReactNode
  children: ReactNode
  wide?: boolean
}) {
  return (
    <div className="overlay" onClick={onClose}>
      <div className={`drawer card ${wide ? 'wide' : ''}`} onClick={(e) => e.stopPropagation()}>
        <div className="drawerhead">
          <h2>{title}</h2>
          <button className="btn ghost icon" onClick={onClose} aria-label="Close">
            ✕
          </button>
        </div>
        <div className="drawerbody">{children}</div>
        {footer && <div className="drawerfoot">{footer}</div>}
      </div>
    </div>
  )
}
