import type { SpectatorRun } from '../shared/api/spectator'

export type ReplaySort = 'recent' | 'runtime' | 'badges'

export function replayRuns(runs: SpectatorRun[]): SpectatorRun[] {
  return runs
    .filter((run) => run.status === 'done' && run.replay_ready)
    .sort((a, b) => Number(b.ended_at || 0) - Number(a.ended_at || 0))
}

export function replayRuntimeSeconds(run: SpectatorRun): number {
  const started = Number(run.queued_at || 0)
  const ended = Number(run.ended_at || 0)
  return started > 0 && ended >= started ? ended - started : 0
}

export function replayResult(run: SpectatorRun): string {
  return (run.highlight || run.reason || 'completed').replaceAll('_', ' ')
}

export function replaySearchText(run: SpectatorRun): string {
  return [
    run.run_id,
    run.llm_profile,
    run.play_style,
    run.starter,
    run.dest,
    run.goal,
    run.stats?.goal_summary,
    replayResult(run),
    ...(run.player?.badges || []),
    ...(run.player?.party || []).map((mon) => mon.name)
  ].filter(Boolean).join(' ').toLowerCase()
}

export function sortReplays(runs: SpectatorRun[], sort: ReplaySort): SpectatorRun[] {
  const out = [...runs]
  if (sort === 'runtime') {
    return out.sort((a, b) => replayRuntimeSeconds(a) - replayRuntimeSeconds(b))
  }
  if (sort === 'badges') {
    return out.sort((a, b) => {
      const badgeDiff = Number(b.player?.badges?.length || 0) - Number(a.player?.badges?.length || 0)
      return badgeDiff || Number(b.ended_at || 0) - Number(a.ended_at || 0)
    })
  }
  return out.sort((a, b) => Number(b.ended_at || 0) - Number(a.ended_at || 0))
}

export function formatReplayDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '—'
  const total = Math.floor(seconds)
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const secs = total % 60
  if (hours > 0) return `${hours}h ${minutes}m`
  if (minutes > 0) return `${minutes}m ${secs}s`
  return `${secs}s`
}
