import { useState } from 'react'
import { api } from '../api'
import { flattenTree } from '../utils'

function toDraft(tracking) {
  const rows = []
  for (const assignment of tracking.assignments) {
    const row = rows.find(row => row.projectId === String(assignment.project_id))
    if (row) row.workspaces += `,${assignment.workspace}`
    else rows.push({ projectId: String(assignment.project_id), workspaces: assignment.workspace })
  }
  return { mode: tracking.mode, rows }
}

export default function TrackingControls({ projects, tracking, onTrackingChange, onRefresh }) {
  // Only edits create a draft. Polls update live status without replacing input.
  const [draft, setDraft] = useState(null)
  const [pending, setPending] = useState('')
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const flat = flattenTree(projects)

  if (!tracking) {
    return (
      <section className="card tracking-controls" aria-labelledby="tracking-heading">
        <h2 id="tracking-heading">Tracking settings</h2>
        <p className="text-muted text-sm">Waiting for tracking configuration.</p>
        <button className="btn btn-ghost btn-sm mt-8" onClick={onRefresh}>Retry</button>
      </section>
    )
  }

  const form = draft ?? toDraft(tracking)
  const sway = tracking.mode === 'sway'

  function edit(next) {
    setDraft(next)
    setError('')
    setMessage('')
  }

  function editRow(index, change) {
    edit({ ...form, rows: form.rows.map((row, i) => i === index ? { ...row, ...change } : row) })
  }

  async function save(event) {
    event.preventDefault()
    setError('')
    setMessage('')
    const assignments = []
    const seen = new Set()
    for (const row of form.rows) {
      const workspaces = row.workspaces.split(',').map(workspace => workspace.trim())
      if (!flat.some(project => String(project.id) === row.projectId) || workspaces.some(workspace => !workspace)) {
        setError('Choose a project and enter comma-separated workspaces for every row, or remove the row.')
        return
      }
      for (const workspace of workspaces) {
        if (seen.has(workspace)) {
          setError(`Workspace "${workspace}" is assigned more than once.`)
          return
        }
        seen.add(workspace)
        assignments.push({ workspace, project_id: Number(row.projectId) })
      }
    }
    setPending('save')
    try {
      const saved = await api.setTracking({ mode: form.mode, enabled: tracking.enabled, assignments })
      onTrackingChange(saved)
      setDraft(null)
      setMessage(`Saved. ${saved.mode === 'sway' ? `Sway tracking is ${saved.enabled ? 'enabled' : 'paused'}.` : 'Global mode uses manual project starts.'}`)
      await onRefresh()
    } catch (error) {
      setError(error.message)
    } finally {
      setPending('')
    }
  }

  async function toggleTracking() {
    setPending('toggle')
    setError('')
    setMessage('')
    try {
      if (tracking.enabled) {
        await api.stop()
        onTrackingChange({ ...tracking, enabled: false })
      } else {
        const saved = await api.setTracking({ mode: tracking.mode, enabled: true, assignments: tracking.assignments })
        onTrackingChange(saved)
      }
      setMessage(tracking.enabled ? 'All workspace tracking paused.' : 'Workspace tracking resumed. Only assigned workspaces are tracked.')
      await onRefresh()
    } catch (error) {
      setError(error.message)
    } finally {
      setPending('')
    }
  }

  return (
    <section className="card tracking-controls" aria-labelledby="tracking-heading">
      <div className="tracking-toolbar">
        <h2 id="tracking-heading">Tracking settings</h2>
        <span className="tag tag-blue">{sway ? 'Sway mode' : 'Global mode'}</span>
      </div>
      {sway && (
        <div className="tracking-live">
          <div className="tracking-toolbar">
            <div className="text-sm">
              <span className={`tag ${tracking.connected ? 'tag-green' : 'tag-warning'}`}>
                {tracking.connected ? 'Sway connected' : 'Sway disconnected'}
              </span>
              <p className="mt-8">Focused workspace: <strong>{tracking.focused_workspace || 'Unknown'}</strong></p>
              <p className="text-muted mt-8">{!tracking.enabled ? 'Tracking is paused.' : !tracking.connected ? 'Waiting for a Sway connection.' : 'Tracking assigned workspaces automatically.'}</p>
            </div>
            <button type="button" className={`btn ${tracking.enabled ? 'btn-stop' : 'btn-start'}`} onClick={toggleTracking} disabled={!!pending}>
              {pending === 'toggle' ? 'Updating...' : tracking.enabled ? 'Pause all tracking' : 'Resume tracking'}
            </button>
          </div>
          {!tracking.connected && <p className="tracking-warning text-sm mt-8" role="status">Sway is unavailable. Check that Sway is running and the server can connect to its IPC socket.</p>}
          {tracking.error && <p className="tracking-error text-sm mt-8" role="alert">{tracking.error}</p>}
        </div>
      )}
      <form onSubmit={save}>
        <fieldset className="tracking-fields" disabled={!!pending}>
          <label className="tracking-field">
            <span>Tracking mode</span>
            <select value={form.mode} onChange={event => edit({ ...form, mode: event.target.value })}>
              <option value="global">Global / manual</option>
              <option value="sway">Sway workspaces</option>
            </select>
          </label>
          <p className="text-muted text-sm mt-8">
            {form.mode === 'sway' ? 'Track projects by focused workspace. Unassigned workspaces are not tracked. Save to apply changes.' : 'Start and stop projects manually. Workspace assignments are kept when switching modes.'}
          </p>
          {(form.mode === 'sway' || form.rows.length > 0) && (
            <div className="mt-16">
              <p className="text-muted text-sm" style={{ marginBottom: 8 }}>Sway workspace assignments</p>
              {form.rows.map((row, index) => (
                <div className="tracking-assignment" key={index}>
                  <label className="tracking-field">
                    <span>Project</span>
                    <select value={row.projectId} onChange={event => editRow(index, { projectId: event.target.value })}>
                      <option value="">Choose a project</option>
                      {row.projectId && !flat.some(project => String(project.id) === row.projectId) && <option value={row.projectId}>Unavailable project #{row.projectId}</option>}
                      {flat.map(project => <option key={project.id} value={project.id}>{'\u00a0\u00a0'.repeat(project.depth)}{project.name}</option>)}
                    </select>
                  </label>
                  <label className="tracking-field">
                    <span>Workspaces</span>
                    <input value={row.workspaces} onChange={event => editRow(index, { workspaces: event.target.value })} placeholder={index === 0 ? '8,9' : '6,7'} />
                  </label>
                  <button type="button" className="btn btn-ghost btn-sm" onClick={() => edit({ ...form, rows: form.rows.filter((_, i) => i !== index) })} aria-label={`Remove assignment ${index + 1}`}>Remove</button>
                </div>
              ))}
              {flat.length === 0 && <p className="text-muted text-sm">Create a project in the Projects tab to assign workspaces.</p>}
              <button type="button" className="btn btn-ghost btn-sm mt-8" disabled={flat.length === 0} onClick={() => edit({ ...form, rows: [...form.rows, { projectId: '', workspaces: '' }] })}>+ Add assignment</button>
            </div>
          )}
          <div className="tracking-actions mt-16">
            <button type="submit" className="btn btn-primary" disabled={!draft}>{pending === 'save' ? 'Saving...' : 'Save settings'}</button>
            {draft && <button type="button" className="btn btn-ghost" onClick={() => { setDraft(null); setError(''); setMessage('') }}>Discard changes</button>}
            {draft && <span className="text-muted text-sm">Unsaved changes. Live controls still use {sway ? 'Sway' : 'global'} mode.</span>}
          </div>
        </fieldset>
      </form>
      {error && <p className="tracking-error text-sm mt-8" role="alert">{error}</p>}
      {message && <p className="text-muted text-sm mt-8" role="status">{message}</p>}
    </section>
  )
}
