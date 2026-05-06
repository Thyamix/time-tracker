import { useState } from 'react'
import { api } from '../api'
import { fmtDuration, fmtDateTime } from '../utils'

function SessionEditor({ session, onSave, onCancel }) {
  const toLocal = (d) => {
    if (!d) return ''
    const dt = new Date(d)
    dt.setMinutes(dt.getMinutes() - dt.getTimezoneOffset())
    return dt.toISOString().slice(0, 16)
  }
  const [start, setStart] = useState(toLocal(session?.start))
  const [end, setEnd] = useState(toLocal(session?.end))
  const [note, setNote] = useState(session?.note || '')

  async function save() {
    const data = {
      start: new Date(start).toISOString(),
      end: end ? new Date(end).toISOString() : null,
      note,
    }
    if (session?.id) {
      await api.updateSession(session.id, data)
    } else {
      await api.createSession({ ...data, project_id: session.project_id })
    }
    onSave()
  }

  return (
    <div style={{ background: 'var(--bg)', border: '1px solid var(--border)', borderRadius: 6, padding: 12, marginTop: 8 }}>
      <div className="grid-2" style={{ gap: 8, marginBottom: 8 }}>
        <div>
          <label className="text-muted text-sm">Start</label>
          <input type="datetime-local" value={start} onChange={e => setStart(e.target.value)} style={{ marginTop: 4 }} />
        </div>
        <div>
          <label className="text-muted text-sm">End</label>
          <input type="datetime-local" value={end} onChange={e => setEnd(e.target.value)} style={{ marginTop: 4 }} />
        </div>
      </div>
      <div style={{ marginBottom: 8 }}>
        <label className="text-muted text-sm">Note</label>
        <input value={note} onChange={e => setNote(e.target.value)} placeholder="Optional note..." style={{ marginTop: 4 }} />
      </div>
      <div className="flex gap-8">
        <button className="btn btn-primary btn-sm" onClick={save}>Save</button>
        <button className="btn btn-ghost btn-sm" onClick={onCancel}>Cancel</button>
      </div>
    </div>
  )
}

function ProjectNode({ project, depth = 0, status, onRefresh, allProjects }) {
  const [open, setOpen] = useState(depth < 1)
  const [showSessions, setShowSessions] = useState(false)
  const [sessions, setSessions] = useState(null)
  const [editingSession, setEditingSession] = useState(null)
  const [newSession, setNewSession] = useState(false)
  const [editingName, setEditingName] = useState(false)
  const [nameVal, setNameVal] = useState(project.name)
  const [addingChild, setAddingChild] = useState(false)
  const [childName, setChildName] = useState('')

  const hasChildren = project.children?.length > 0
  const isActive = status?.session?.project_id === project.id

  async function loadSessions() {
    const s = await api.getSessions({ project_id: project.id })
    setSessions(s || [])
  }

  async function toggleSessions() {
    if (!showSessions) await loadSessions()
    setShowSessions(s => !s)
  }

  async function deleteProject() {
    if (!confirm(`Delete "${project.name}" and all its sessions?`)) return
    await api.deleteProject(project.id)
    onRefresh()
  }

  async function saveName() {
    await api.updateProject(project.id, nameVal, project.parent_id)
    setEditingName(false)
    onRefresh()
  }

  async function addChild() {
    if (!childName.trim()) return
    await api.createProject(childName.trim(), project.id)
    setChildName('')
    setAddingChild(false)
    onRefresh()
  }

  async function deleteSession(id) {
    if (!confirm('Delete this session?')) return
    await api.deleteSession(id)
    await loadSessions()
    onRefresh()
  }

  async function start() {
    await api.start(project.id)
    onRefresh()
  }

  return (
    <div style={{ marginLeft: depth * 20 }}>
      <div className="flex items-center gap-8"
        style={{
          padding: '6px 8px', borderRadius: 6,
          background: isActive ? 'rgba(63,185,80,0.08)' : 'transparent'
        }}
      >
        {/* Expand toggle */}
        <button
          className="btn btn-ghost btn-sm"
          style={{ width: 22, padding: 0, visibility: hasChildren ? 'visible' : 'hidden' }}
          onClick={() => setOpen(o => !o)}
        >
          {open ? '▾' : '▸'}
        </button>

        {/* Name */}
        {editingName ? (
          <input
            autoFocus value={nameVal}
            onChange={e => setNameVal(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') saveName(); if (e.key === 'Escape') setEditingName(false) }}
            style={{ width: 160 }}
          />
        ) : (
          <span style={{ fontWeight: hasChildren ? 600 : 400, cursor: 'pointer' }}
            onDoubleClick={() => setEditingName(true)}
          >
            {project.name}
          </span>
        )}

        <span className="text-muted text-sm">{fmtDuration(project.total_seconds)}</span>
        {isActive && <span className="tag tag-green" style={{ fontSize: 10 }}>● live</span>}

        <div className="flex gap-8" style={{ marginLeft: 'auto' }}>
          <button className="btn btn-sm btn-start" onClick={start} disabled={isActive} title="Start tracking">▶</button>
          <button className="btn btn-sm btn-ghost" onClick={() => setAddingChild(a => !a)} title="Add child">+</button>
          <button className="btn btn-sm btn-ghost" onClick={toggleSessions} title="Sessions">
            {showSessions ? '▲' : '≡'}
          </button>
          <button className="btn btn-sm btn-danger" onClick={deleteProject} title="Delete">✕</button>
        </div>
      </div>

      {/* Add child form */}
      {addingChild && (
        <div className="flex gap-8" style={{ marginLeft: 30, marginTop: 4, marginBottom: 4 }}>
          <input autoFocus value={childName} onChange={e => setChildName(e.target.value)}
            placeholder="Child project name..."
            onKeyDown={e => { if (e.key === 'Enter') addChild(); if (e.key === 'Escape') setAddingChild(false) }}
          />
          <button className="btn btn-primary btn-sm" onClick={addChild}>Add</button>
          <button className="btn btn-ghost btn-sm" onClick={() => setAddingChild(false)}>Cancel</button>
        </div>
      )}

      {/* Sessions panel */}
      {showSessions && (
        <div style={{ marginLeft: 30, marginBottom: 8 }}>
          <div style={{ background: 'var(--bg)', border: '1px solid var(--border)', borderRadius: 6, padding: 10 }}>
            {(sessions || []).length === 0 && <div className="text-muted text-sm">No sessions yet.</div>}
            {(sessions || []).map(s => (
              <div key={s.id}>
                {editingSession?.id === s.id ? (
                  <SessionEditor session={s} onSave={() => { setEditingSession(null); loadSessions(); onRefresh() }} onCancel={() => setEditingSession(null)} />
                ) : (
                  <div className="flex items-center justify-between" style={{ padding: '4px 0', borderBottom: '1px solid var(--border)' }}>
                    <div>
                      <span className="text-sm">{fmtDateTime(s.start)}</span>
                      {s.end && <span className="text-muted text-sm"> → {fmtDateTime(s.end)}</span>}
                      <span className="text-muted text-sm" style={{ marginLeft: 8 }}>{fmtDuration(s.duration)}</span>
                      {s.note && <span className="text-muted text-sm" style={{ marginLeft: 8 }}>— {s.note}</span>}
                    </div>
                    <div className="flex gap-8">
                      <button className="btn btn-ghost btn-sm" onClick={() => setEditingSession(s)}>Edit</button>
                      <button className="btn btn-danger btn-sm" onClick={() => deleteSession(s.id)}>✕</button>
                    </div>
                  </div>
                )}
              </div>
            ))}
            <div style={{ marginTop: 8 }}>
              {newSession ? (
                <SessionEditor
                  session={{ project_id: project.id }}
                  onSave={() => { setNewSession(false); loadSessions(); onRefresh() }}
                  onCancel={() => setNewSession(false)}
                />
              ) : (
                <button className="btn btn-ghost btn-sm" onClick={() => setNewSession(true)}>+ Add session</button>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Children */}
      {open && project.children?.map(child => (
        <ProjectNode key={child.id} project={child} depth={depth + 1} status={status} onRefresh={onRefresh} allProjects={allProjects} />
      ))}
    </div>
  )
}

export default function Projects({ projects, status, onRefresh }) {
  const [newName, setNewName] = useState('')

  async function createRoot() {
    if (!newName.trim()) return
    await api.createProject(newName.trim(), null)
    setNewName('')
    onRefresh()
  }

  return (
    <div>
      <h1>Projects</h1>

      {/* New root project */}
      <div className="card" style={{ marginBottom: 16 }}>
        <div className="flex gap-8">
          <input
            value={newName}
            onChange={e => setNewName(e.target.value)}
            placeholder="New root project name..."
            onKeyDown={e => e.key === 'Enter' && createRoot()}
          />
          <button className="btn btn-primary" onClick={createRoot}>Add</button>
        </div>
      </div>

      <div className="card">
        {projects.length === 0 ? (
          <div className="text-muted text-sm">No projects yet.</div>
        ) : (
          projects.map(p => (
            <ProjectNode key={p.id} project={p} depth={0} status={status} onRefresh={onRefresh} allProjects={projects} />
          ))
        )}
      </div>
    </div>
  )
}
