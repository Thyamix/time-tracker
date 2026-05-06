import { useState, useEffect } from 'react'
import { api } from '../api'
import { fmtDuration, flattenTree } from '../utils'

export default function Reports({ projects }) {
  const [range, setRange] = useState('alltime')
  const [stats, setStats] = useState(null)

  useEffect(() => {
    api.getStats(range).then(setStats)
  }, [range])

  const flat = flattenTree(stats?.projects || [])

  return (
    <div>
      <h1>Reports</h1>
      <div className="card" style={{ marginBottom: 16 }}>
        <div className="flex gap-8">
          {[
            { id: 'today', label: 'Today' },
            { id: 'week', label: 'Last 7 days' },
            { id: 'month', label: 'Last 30 days' },
            { id: 'alltime', label: 'All time' },
          ].map(r => (
            <button
              key={r.id}
              className={`btn ${range === r.id ? 'btn-primary' : 'btn-ghost'}`}
              onClick={() => setRange(r.id)}
            >
              {r.label}
            </button>
          ))}
        </div>
      </div>
      <div className="card">
        {flat.length === 0 ? (
          <div className="text-muted text-sm">No data for this period.</div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            {flat.map(p => (
              <div
                key={p.id}
                className="flex items-center justify-between"
                style={{ paddingLeft: p.depth * 20 }}
              >
                <span style={{ fontWeight: p.children?.length ? 600 : 400 }}>{p.name}</span>
                <span className="text-muted text-sm">{fmtDuration(p.total_seconds)}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
