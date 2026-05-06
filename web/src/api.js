const BASE = 'http://localhost:7332/api'

async function req(method, path, body) {
  const res = await fetch(BASE + path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
  })
  if (res.status === 204) return null
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'Request failed')
  return data
}

export const api = {
  // Status
  status: () => req('GET', '/status'),

  // Projects
  getProjects: () => req('GET', '/projects'),
  createProject: (name, parent_id) => req('POST', '/projects', { name, parent_id }),
  updateProject: (id, name, parent_id) => req('PATCH', `/projects/${id}`, { name, parent_id }),
  deleteProject: (id) => req('DELETE', `/projects/${id}`),

  // Sessions
  getSessions: (params = {}) => {
    const q = new URLSearchParams(params).toString()
    return req('GET', `/sessions${q ? '?' + q : ''}`)
  },
  createSession: (data) => req('POST', '/sessions', data),
  updateSession: (id, data) => req('PATCH', `/sessions/${id}`, data),
  deleteSession: (id) => req('DELETE', `/sessions/${id}`),

  // Tracking
  start: (project_id) => req('POST', '/track/start', { project_id }),
  stop: (note = '') => req('POST', '/track/stop', { note }),

  // Stats
  getStats: (range = 'alltime') => req('GET', `/stats?range=${range}`),

  // Import
  importSessions: (sessions) => req('POST', '/import', sessions),
}
