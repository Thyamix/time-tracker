import { useState, useEffect, useCallback } from 'react'
import { api } from './api'
import Dashboard from './views/Dashboard'
import Projects from './views/Projects'
import Reports from './views/Reports'
import Import from './views/Import'
import './App.css'

export default function App() {
  const [view, setView] = useState('dashboard')
  const [projects, setProjects] = useState([])
  const [status, setStatus] = useState(null)

  const refresh = useCallback(async () => {
    try {
      const [p, s] = await Promise.all([api.getProjects(), api.status()])
      setProjects(p || [])
      setStatus(s)
    } catch (e) { /* server may be starting */ }
  }, [])

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, 5000)
    return () => clearInterval(t)
  }, [refresh])

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
              <div className="status-time">{status.text?.split('  ')[1]}</div>
            </div>
          </div>
        )}
      </aside>
      <main className="content">
        {view === 'dashboard' && <Dashboard projects={projects} status={status} onRefresh={refresh} />}
        {view === 'projects' && <Projects projects={projects} status={status} onRefresh={refresh} />}
        {view === 'reports' && <Reports projects={projects} />}
        {view === 'import' && <Import onRefresh={refresh} />}
      </main>
    </div>
  )
}
