import { useState, useEffect } from 'react'
import { api } from '../api'
import { fmtDuration, flattenTree } from '../utils'

export default function Reports() {
  const [range, setRange] = useState('alltime')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [month, setMonth] = useState('')
  const [stats, setStats] = useState(null)

  const applyDateRange = () => {
    if (!from || !to || from > to) return
    setRange({ type: 'dates', from, to })
    setMonth('')
  }

  const applyMonth = () => {
    if (!month) return
    setRange({ type: 'month', month })
    setFrom('')
    setTo('')
  }

  useEffect(() => {
    const params = typeof range === 'string'
      ? { range }
      : range.type === 'month'
        ? { range: 'calendar-month', month: range.month }
        : { range: 'custom', from: range.from, to: range.to }
    api.getStats(params).then(setStats)
  }, [range])

  const flat = flattenTree(stats?.projects || [])

  return (
    <div>
      <h1>Reports</h1>
      <div className="card" style={{ marginBottom: 16 }}>
        <div className="flex gap-8" style={{ flexWrap: 'wrap' }}>
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
        <div className="report-filters">
          <div className="report-filter">
            <label htmlFor="report-from">Date range</label>
            <div className="flex gap-8 items-center">
              <input id="report-from" type="date" value={from} onChange={e => setFrom(e.target.value)} />
              <span className="text-muted">to</span>
              <input aria-label="End date" type="date" value={to} onChange={e => setTo(e.target.value)} />
              <button className="btn btn-ghost" onClick={applyDateRange} disabled={!from || !to || from > to}>Apply</button>
            </div>
          </div>
          <div className="report-filter">
            <label htmlFor="report-month">Specific month</label>
            <div className="flex gap-8 items-center">
              <input id="report-month" type="month" value={month} onChange={e => setMonth(e.target.value)} />
              <button className="btn btn-ghost" onClick={applyMonth} disabled={!month}>Apply</button>
            </div>
          </div>
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
