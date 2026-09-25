export interface SelectableSpectatorRun {
  run_id: string
  status: string
  queued_at?: number
  ended_at?: number
  featured?: boolean
}

function newest<T extends SelectableSpectatorRun>(runs: T[], field: 'queued_at' | 'ended_at'): T | null {
  return [...runs].sort((a, b) => Number(b[field] || 0) - Number(a[field] || 0))[0] || null
}

export function preferredRun<T extends SelectableSpectatorRun>(runs: T[], selectedRunID = ''): T | null {
  if (!runs.length) return null
  if (selectedRunID) {
    return runs.find((run) => run.run_id === selectedRunID) || null
  }
  return runs.find((run) => run.featured)
    || newest(runs.filter((run) => run.status === 'running'), 'queued_at')
    || newest(runs.filter((run) => run.status === 'leased'), 'queued_at')
    || newest(runs.filter((run) => run.status === 'paused'), 'queued_at')
    || newest(runs.filter((run) => run.status === 'queued'), 'queued_at')
    || newest(runs.filter((run) => run.status === 'done'), 'ended_at')
    || runs[runs.length - 1]
    || null
}
