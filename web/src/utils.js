export function fmtDuration(totalSeconds) {
  if (!totalSeconds && totalSeconds !== 0) return ''
  const h = Math.floor(totalSeconds / 3600)
  const m = Math.floor((totalSeconds % 3600) / 60)
  const s = totalSeconds % 60
  if (h > 0) {
    return `${h}h ${String(m).padStart(2, '0')}m ${String(s).padStart(2, '0')}s`
  }
  return `${m}m ${String(s).padStart(2, '0')}s`
}

export function fmtDateTime(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  return d.toLocaleString()
}

export function flattenTree(projects, depth = 0) {
  const result = []
  for (const p of projects || []) {
    result.push({ ...p, depth })
    if (p.children?.length) {
      result.push(...flattenTree(p.children, depth + 1))
    }
  }
  return result
}
