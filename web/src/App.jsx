import { useState, useEffect, useCallback } from 'react'
import { api } from './api'
import { fmtDuration } from './utils'
import Dashboard from './views/Dashboard'
import Projects from './views/Projects'
import Reports from './views/Reports'
import Import from './views/Import'
import './App.css'

export default function App() {
  const [view, setView] = useState('dashboard')
  const [projects, setProjects] = useState([])
  const [status, setStatus] = useState(null)
  const [statusError, setStatusError] = useState('')

  const refresh = useCallback(() => {
    return Promise.all([api.getProjects(), api.status()]).then(([p, s]) => {
      setProjects(p || [])
      setStatus(s)
      setStatusError('')
    }).catch(error => setStatusError(error.message))
  }, [])

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, 5000)
    return () => clearInterval(t)
  }, [refresh])

  function updateTracking(tracking) {
    setStatus(current => current ? { ...current, tracking } : current)
  }

  const nav = [
    { id: 'dashboard', label: '⏱ Dashboard' },
    { id: 'projects', label: '📁 Projects' },
    { id: 'reports', label: '📊 Reports' },
    { id: 'import', label: '📥 Import' },
  ]

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="logo">timetrack</div>
        <nav>
          {nav.map(n => (
            <button
              key={n.id}
              className={`nav-btn ${view === n.id ? 'active' : ''}`}
              onClick={() => setView(n.id)}
            >
              {n.label}
            </button>
          ))}
        </nav>
        {status?.active && (
          <div className="sidebar-status">
            <div className="status-dot" />
            <div className="status-info">
              <div className="status-path">{status.path}</div>
              <div className="status-time">{fmtDuration(status.elapsed_seconds)} ({fmtDuration(status.total_seconds)})</div>
            </div>
          </div>
        )}
      </aside>
      <main className="content">
        {statusError && <div className="card tracking-error" role="alert" style={{ marginBottom: 16 }}>Could not refresh status: {statusError}. Displayed status may be out of date.</div>}
        {view === 'dashboard' && <Dashboard projects={projects} status={status} onRefresh={refresh} onTrackingChange={updateTracking} />}
        {view === 'projects' && <Projects projects={projects} status={status} onRefresh={refresh} />}
        {view === 'reports' && <Reports />}
        {view === 'import' && <Import onRefresh={refresh} />}
      </main>
    </div>
  )
}
