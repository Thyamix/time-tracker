import { useState, useEffect } from 'react'
import { api } from '../api'
import { fmtDuration, fmtDateTime, flattenTree } from '../utils'
import TrackingControls from './TrackingControls'

function SessionTimer({ seconds }) {
  const [elapsed, setElapsed] = useState(seconds || 0)
  useEffect(() => {
    const t = setInterval(() => setElapsed(e => e + 1), 1000)
    return () => clearInterval(t)
  }, [])
  const h = Math.floor(elapsed / 3600)
  const m = Math.floor((elapsed % 3600) / 60)
  const s = elapsed % 60
  return h > 0
    ? `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
    : `${m}:${String(s).padStart(2, '0')}`
}

export default function Dashboard({ projects, status, onRefresh, onTrackingChange }) {
  const [stopNote, setStopNote] = useState('')
  const [showStopNote, setShowStopNote] = useState(false)
  const [recentSessions, setRecentSessions] = useState([])
  const [actionError, setActionError] = useState('')
  const [pending, setPending] = useState(false)
  const sway = status?.tracking?.mode === 'sway'

  useEffect(() => {
    let cancelled = false
    api.getSessions().then(s => {
      if (!cancelled) setRecentSessions((s || []).slice(0, 8))
    }).catch(error => {
      if (!cancelled) setActionError(`Could not load recent sessions: ${error.message}`)
    })
    return () => { cancelled = true }
  }, [status?.active, status?.session?.id])

  const flat = flattenTree(projects)

  async function handleStart(projectId) {
    if (sway || !status) return
    setPending(true)
    setActionError('')
    try {
      await api.start(projectId)
      await onRefresh()
    } catch (error) {
      setActionError(error.message)
    } finally {
      setPending(false)
    }
  }

  async function handleStop() {
    if (!showStopNote) { setShowStopNote(true); return }
    setPending(true)
    setActionError('')
    try {
      await api.stop(stopNote)
      if (sway) onTrackingChange({ ...status.tracking, enabled: false })
      setShowStopNote(false)
      setStopNote('')
      await onRefresh()
    } catch (error) {
      setActionError(error.message)
    } finally {
      setPending(false)
    }
  }

  return (
    <div>
      <h1>Dashboard</h1>

      <TrackingControls projects={projects} tracking={status?.tracking} onTrackingChange={onTrackingChange} onRefresh={onRefresh} />
      {actionError && <p className="tracking-error text-sm" role="alert" style={{ marginBottom: 16 }}>{actionError}</p>}

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
                  <SessionTimer key={`${status.session?.id}:${status.elapsed_seconds}`} seconds={status.elapsed_seconds} />
                </div>
                <div className="text-muted text-sm">total: {fmtDuration(status.total_seconds)}</div>
              </div>
            </div>
            {showStopNote ? (
              <div className="tracking-actions">
                <input
                  autoFocus
                  placeholder="Note (optional)..."
                  value={stopNote}
                  onChange={e => setStopNote(e.target.value)}
                  disabled={pending}
                  onKeyDown={e => e.key === 'Enter' && !pending && handleStop()}
                />
                <button className="btn btn-stop" disabled={pending} onClick={handleStop}>{sway ? 'Pause all tracking' : 'Stop'}</button>
                <button className="btn btn-ghost" disabled={pending} onClick={() => setShowStopNote(false)}>Cancel</button>
              </div>
            ) : (
              <button className="btn btn-stop" disabled={pending} onClick={handleStop}>{sway ? 'Pause all tracking with note' : '⏹ Stop'}</button>
            )}
          </div>
        ) : (
          <div className="text-muted" style={{ textAlign: 'center', padding: '12px 0' }}>
            {sway ? !status.tracking.enabled ? 'Workspace tracking is paused. Resume in tracking settings.' : !status.tracking.connected ? 'Waiting for Sway to connect.' : 'No active session. Focus an assigned workspace to track.' : 'No active session — pick a project below to start'}
          </div>
        )}
      </div>

      {/* Quick start */}
      <h2>Projects</h2>
      {sway && <p className="text-muted text-sm" style={{ marginBottom: 12 }}>Projects start automatically on assigned workspaces. Edit assignments in tracking settings.</p>}
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
                  disabled={!status || sway || pending || status?.session?.project_id === p.id}
                  title={sway ? 'Managed by Sway workspace assignments' : 'Start tracking'}
                  onClick={() => handleStart(p.id)}
                >
                  {sway ? 'Sway' : status?.session?.project_id === p.id ? '●' : '▶'}
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
