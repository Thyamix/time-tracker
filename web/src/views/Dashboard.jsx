import { useState, useEffect } from 'react'
import { api } from '../api'
import { fmtDuration, fmtDateTime, flattenTree } from '../utils'

export default function Dashboard({ projects, status, onRefresh }) {
  const [elapsed, setElapsed] = useState(status?.elapsed_seconds || 0)
  const [stopNote, setStopNote] = useState('')
  const [showStopNote, setShowStopNote] = useState(false)
  const [recentSessions, setRecentSessions] = useState([])

  useEffect(() => {
    api.getSessions().then(s => setRecentSessions((s || []).slice(0, 8)))
  }, [status?.active])

  // Live elapsed timer
  useEffect(() => {
    if (!status?.active) { setElapsed(0); return }
    setElapsed(status.elapsed_seconds)
    const t = setInterval(() => setElapsed(e => e + 1), 1000)
    return () => clearInterval(t)
  }, [status?.active, status?.elapsed_seconds])

  const flat = flattenTree(projects)

  async function handleStart(projectId) {
    await api.start(projectId)
    onRefresh()
  }

  async function handleStop() {
    if (!showStopNote) { setShowStopNote(true); return }
    await api.stop(stopNote)
    setShowStopNote(false)
    setStopNote('')
    onRefresh()
  }

  const h = Math.floor(elapsed / 3600)
  const m = Math.floor((elapsed % 3600) / 60)
  const s = elapsed % 60
  const elapsedStr = h > 0
    ? `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
    : `${m}:${String(s).padStart(2, '0')}`

  return (
    <div>
      <h1>Dashboard</h1>

      {/* Active session card */}
      <div className="card" style={{ marginBottom: 16 }}>
        {status?.active ? (
          <div>
            <div className="flex items-center justify-between" style={{ marginBottom: 12 }}>
              <div>
                <span className="tag tag-green" style={{ marginBottom: 6 }}>● tracking</span>
                <div style={{ fontSize: 22, fontWeight: 700, marginTop: 4 }}>{status.path}</div>
              </div>
              <div style={{ textAlign: 'right' }}>
                <div style={{ fontSize: 32, fontWeight: 700, fontVariantNumeric: 'tabular-nums', color: 'var(--green)' }}>
                  {elapsedStr}
                </div>
                <div className="text-muted text-sm">total: {fmtDuration(status.total_seconds)}</div>
              </div>
            </div>
            {showStopNote ? (
              <div className="flex gap-8">
                <input
                  autoFocus
                  placeholder="Note (optional)..."
                  value={stopNote}
                  onChange={e => setStopNote(e.target.value)}
                  onKeyDown={e => e.key === 'Enter' && handleStop()}
                />
                <button className="btn btn-stop" onClick={handleStop}>Stop</button>
                <button className="btn btn-ghost" onClick={() => setShowStopNote(false)}>Cancel</button>
              </div>
            ) : (
              <button className="btn btn-stop" onClick={handleStop}>⏹ Stop</button>
            )}
          </div>
        ) : (
          <div className="text-muted" style={{ textAlign: 'center', padding: '12px 0' }}>
            No active session — pick a project below to start
          </div>
        )}
      </div>

      {/* Quick start */}
      <h2>Projects</h2>
      <div className="card">
        {flat.length === 0 ? (
          <div className="text-muted text-sm">No projects yet — create one in the Projects tab.</div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            {flat.map(p => (
              <div key={p.id}
                className="flex items-center justify-between"
                style={{
                  padding: '6px 8px', borderRadius: 6,
                  paddingLeft: 8 + p.depth * 20,
                  background: status?.session?.project_id === p.id ? 'rgba(63,185,80,0.08)' : undefined
                }}
              >
                <div className="flex items-center gap-8">
                  {p.depth > 0 && <span className="text-muted">└</span>}
                  <span style={{ fontWeight: p.children?.length ? 600 : 400 }}>{p.name}</span>
                  <span className="text-muted text-sm">{fmtDuration(p.total_seconds)}</span>
                </div>
                <button
                  className="btn btn-sm btn-start"
                  disabled={status?.session?.project_id === p.id}
                  onClick={() => handleStart(p.id)}
                >
                  {status?.session?.project_id === p.id ? '●' : '▶'}
                </button>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Recent sessions */}
      {recentSessions.length > 0 && (
        <div style={{ marginTop: 16 }}>
          <h2>Recent Sessions</h2>
          <div className="card">
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              {recentSessions.map(s => (
                <div key={s.id} className="flex items-center justify-between">
                  <div>
                    <span style={{ fontSize: 13 }}>{fmtDateTime(s.start)}</span>
                    {s.note && <span className="text-muted text-sm" style={{ marginLeft: 8 }}>— {s.note}</span>}
                  </div>
                  <span className="text-muted text-sm">{fmtDuration(s.duration)}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
