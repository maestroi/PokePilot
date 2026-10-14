// These are operator-browser conveniences, not server policy. A run still
// submits an explicit RunSpec and each field can be overridden at launch.
export const RUN_LAUNCH_DEFAULTS_KEY = 'pokepilot.operator.runDefaults'
export type RunLaunchDefaults = {
  playStyle: 'adventure' | 'speedrun' | 'completionist' | 'team_builder'
  fps: 0 | 60 | 120 | 240 | 480
  recoveryProfile: 'resilient' | 'strict'
}

export const DEFAULT_RUN_LAUNCH: RunLaunchDefaults = {
  playStyle: 'adventure',
  fps: 60,
  recoveryProfile: 'resilient'
}

export function parseRunLaunchDefaults(value: unknown): RunLaunchDefaults {
  const data = value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
  const styles = ['adventure', 'speedrun', 'completionist', 'team_builder']
  const speeds = [0, 60, 120, 240, 480]
  return {
    playStyle: styles.includes(String(data.playStyle)) ? data.playStyle as RunLaunchDefaults['playStyle'] : DEFAULT_RUN_LAUNCH.playStyle,
    fps: typeof data.fps === 'number' && speeds.includes(data.fps) ? data.fps as RunLaunchDefaults['fps'] : DEFAULT_RUN_LAUNCH.fps,
    recoveryProfile: data.recoveryProfile === 'strict' ? 'strict' : 'resilient'
  }
}

export function readRunLaunchDefaults(storage?: Pick<Storage, 'getItem'>): RunLaunchDefaults {
  try {
    const source = storage ?? (typeof window !== 'undefined' ? window.localStorage : undefined)
    const raw = source?.getItem(RUN_LAUNCH_DEFAULTS_KEY)
    return raw ? parseRunLaunchDefaults(JSON.parse(raw)) : { ...DEFAULT_RUN_LAUNCH }
  } catch {
    return { ...DEFAULT_RUN_LAUNCH }
  }
}

export function saveRunLaunchDefaults(value: RunLaunchDefaults, storage?: Pick<Storage, 'setItem'>): void {
  const source = storage ?? (typeof window !== 'undefined' ? window.localStorage : undefined)
  if (!source) return
  source.setItem(RUN_LAUNCH_DEFAULTS_KEY, JSON.stringify(parseRunLaunchDefaults(value)))
}
