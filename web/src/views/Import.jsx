import { useState } from 'react'
import { api } from '../api'

export default function Import({ onRefresh }) {
  const [text, setText] = useState('')
  const [message, setMessage] = useState('')

  async function handleImport() {
    setMessage('')
    let sessions
    try {
      sessions = JSON.parse(text)
      if (!Array.isArray(sessions)) throw new Error('Expected an array')
    } catch (e) {
      setMessage('Invalid JSON: ' + e.message)
      return
    }
    try {
      const res = await api.importSessions(sessions)
      setMessage(`Imported ${res.imported} sessions.`)
      setText('')
      onRefresh()
    } catch (e) {
      setMessage('Error: ' + e.message)
    }
  }

  return (
    <div>
      <h1>Import</h1>
      <div className="card">
        <p className="text-muted text-sm" style={{ marginBottom: 12 }}>
          Paste a JSON array of sessions. Each session needs <code>project</code> (string), <code>start</code> (Unix seconds), and <code>end</code> (Unix seconds).
        </p>
        <textarea
          value={text}
          onChange={e => setText(e.target.value)}
          placeholder={`[\n  { "project": "Work", "start": 1700000000, "end": 1700003600 }\n]`}
          rows={12}
          style={{ width: '100%', fontFamily: 'monospace', fontSize: 13 }}
        />
        <div className="flex gap-8" style={{ marginTop: 12 }}>
          <button className="btn btn-primary" onClick={handleImport}>Import</button>
          {message && <span className="text-muted">{message}</span>}
        </div>
      </div>
    </div>
  )
}
