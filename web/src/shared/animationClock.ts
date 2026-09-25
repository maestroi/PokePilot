import type { RenderActor, RenderPosition, RenderState } from './api/renderstate'

export type AnimationDiscontinuity =
  | 'initial'
  | 'none'
  | 'map-change'
  | 'teleport'
  | 'reconnect'
  | 'rewind'

export interface AnimationClockOptions {
  minTweenMs?: number
  maxTweenMs?: number
  reconnectGapMs?: number
  teleportDistanceTiles?: number
  movingTeleportDistanceTiles?: number
  entityTeleportDistanceTiles?: number
}

export interface PresentationSample {
  player: RenderPosition | null
  camera: RenderPosition | null
  entityPositions: Map<string, RenderPosition>
  animating: boolean
  discontinuity: AnimationDiscontinuity
  sourceFrame: number
  sourceCapturedAtUnixMS?: number
}

interface Tween {
  from: RenderPosition
  to: RenderPosition
  startedAt: number
  durationMs: number
}

const DEFAULT_MIN_TWEEN_MS = 55
const DEFAULT_MAX_TWEEN_MS = 140
const DEFAULT_RECONNECT_GAP_MS = 1200
const DEFAULT_TELEPORT_DISTANCE = 12
const DEFAULT_MOVING_TELEPORT_DISTANCE = 24
const DEFAULT_ENTITY_TELEPORT_DISTANCE = 6

function point(position: RenderPosition): RenderPosition {
  return { x: Number(position.x || 0), y: Number(position.y || 0) }
}

function distance(a: RenderPosition, b: RenderPosition): number {
  return Math.abs(a.x - b.x) + Math.abs(a.y - b.y)
}

function samePoint(a: RenderPosition, b: RenderPosition): boolean {
  return a.x === b.x && a.y === b.y
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value))
}

function smoothstep(progress: number): number {
  const p = clamp(progress, 0, 1)
  return p * p * (3 - 2 * p)
}

function sampleTween(tween: Tween | null, now: number): RenderPosition | null {
  if (!tween) return null
  if (tween.durationMs <= 0 || now >= tween.startedAt + tween.durationMs) return point(tween.to)
  if (now <= tween.startedAt) return point(tween.from)
  const progress = smoothstep((now - tween.startedAt) / tween.durationMs)
  return {
    x: tween.from.x + (tween.to.x - tween.from.x) * progress,
    y: tween.from.y + (tween.to.y - tween.from.y) * progress
  }
}

function tweenActive(tween: Tween | null, now: number): boolean {
  return Boolean(tween && !samePoint(tween.from, tween.to) && now < tween.startedAt + tween.durationMs)
}

function actorKey(actor: RenderActor, index: number): string {
  return actor.id || `${actor.kind || 'entity'}:${index}`
}

function authoritativePosition(actor: RenderActor): RenderPosition {
  const movement = actor.movement
  const progress = Number(movement?.progress)
  if (
    movement &&
    Number.isFinite(progress) &&
    progress > 0 &&
    progress <= 1
  ) {
    return {
      x: movement.from.x + (movement.to.x - movement.from.x) * progress,
      y: movement.from.y + (movement.to.y - movement.from.y) * progress
    }
  }
  return point(actor.position)
}

function moving(actor: RenderActor): boolean {
  const kind = (actor.movement?.kind || '').toLowerCase()
  return Boolean(kind && kind !== 'idle' && kind !== 'unknown')
}

/**
 * PresentationClock converts sparse authoritative RenderState snapshots into a
 * bounded visual timeline. It owns no timers: callers pass explicit arrival
 * and sample times, which keeps replay/tests deterministic and guarantees that
 * interpolated positions can never leak back into gameplay state.
 */
export class PresentationClock {
  private readonly minTweenMs: number
  private readonly maxTweenMs: number
  private readonly reconnectGapMs: number
  private readonly teleportDistanceTiles: number
  private readonly movingTeleportDistanceTiles: number
  private readonly entityTeleportDistanceTiles: number

  private mapID = ''
  private sourceFrame = -1
  private sourceCapturedAtUnixMS: number | undefined
  private lastReceivedAt = Number.NaN
  private playerTween: Tween | null = null
  private cameraTween: Tween | null = null
  private entityTweens = new Map<string, Tween>()
  private discontinuity: AnimationDiscontinuity = 'initial'

  constructor(options: AnimationClockOptions = {}) {
    this.minTweenMs = Math.max(0, options.minTweenMs ?? DEFAULT_MIN_TWEEN_MS)
    this.maxTweenMs = Math.max(this.minTweenMs, options.maxTweenMs ?? DEFAULT_MAX_TWEEN_MS)
    this.reconnectGapMs = Math.max(this.maxTweenMs, options.reconnectGapMs ?? DEFAULT_RECONNECT_GAP_MS)
    this.teleportDistanceTiles = Math.max(1, options.teleportDistanceTiles ?? DEFAULT_TELEPORT_DISTANCE)
    this.movingTeleportDistanceTiles = Math.max(
      this.teleportDistanceTiles,
      options.movingTeleportDistanceTiles ?? DEFAULT_MOVING_TELEPORT_DISTANCE
    )
    this.entityTeleportDistanceTiles = Math.max(1, options.entityTeleportDistanceTiles ?? DEFAULT_ENTITY_TELEPORT_DISTANCE)
  }

  reset(): void {
    this.mapID = ''
    this.sourceFrame = -1
    this.sourceCapturedAtUnixMS = undefined
    this.lastReceivedAt = Number.NaN
    this.playerTween = null
    this.cameraTween = null
    this.entityTweens.clear()
    this.discontinuity = 'initial'
  }

  ingest(state: RenderState, receivedAtMs: number): AnimationDiscontinuity {
    const player = state.player
    const mapID = state.map?.id || ''
    if (!player || !mapID) {
      this.reset()
      return this.discontinuity
    }

    const frame = Number(state.clock.frame || 0)
    const target = authoritativePosition(player)
    const receivedAt = Number.isFinite(receivedAtMs) ? receivedAtMs : 0

    if (this.sourceFrame < 0) {
      this.snap(state, target, receivedAt, 'initial')
      return this.discontinuity
    }

    const arrivalGap = Math.max(0, receivedAt - this.lastReceivedAt)
    let discontinuity: AnimationDiscontinuity = 'none'
    if (frame < this.sourceFrame) discontinuity = 'rewind'
    else if (mapID !== this.mapID) discontinuity = 'map-change'
    else if (arrivalGap > this.reconnectGapMs) discontinuity = 'reconnect'
    else {
      const currentPlayer = sampleTween(this.playerTween, receivedAt) || target
      const threshold = moving(player) ? this.movingTeleportDistanceTiles : this.teleportDistanceTiles
      if (distance(currentPlayer, target) > threshold) discontinuity = 'teleport'
    }

    if (discontinuity !== 'none') {
      this.snap(state, target, receivedAt, discontinuity)
      return this.discontinuity
    }

    // Repeated authoritative frames are a pause/stall heartbeat. Refresh the
    // arrival time so a later resume does not accumulate artificial catch-up.
    if (frame === this.sourceFrame) {
      this.lastReceivedAt = receivedAt
      this.sourceCapturedAtUnixMS = state.clock.captured_at_unix_ms
      this.discontinuity = 'none'
      return this.discontinuity
    }

    const durationMs = this.tweenDuration(arrivalGap)
    const currentPlayer = sampleTween(this.playerTween, receivedAt) || target
    const currentCamera = sampleTween(this.cameraTween, receivedAt) || currentPlayer

    this.playerTween = this.makeTween(currentPlayer, target, receivedAt, durationMs)
    this.cameraTween = this.makeTween(
      currentCamera,
      target,
      receivedAt,
      Math.min(this.maxTweenMs, Math.max(durationMs, durationMs * 1.2))
    )
    this.retargetEntities(state.entities || [], receivedAt, durationMs)

    this.mapID = mapID
    this.sourceFrame = frame
    this.sourceCapturedAtUnixMS = state.clock.captured_at_unix_ms
    this.lastReceivedAt = receivedAt
    this.discontinuity = 'none'
    return this.discontinuity
  }

  sample(nowMs: number): PresentationSample {
    const now = Number.isFinite(nowMs) ? nowMs : 0
    const entityPositions = new Map<string, RenderPosition>()
    let animating = tweenActive(this.playerTween, now) || tweenActive(this.cameraTween, now)

    for (const [key, tween] of this.entityTweens) {
      const position = sampleTween(tween, now)
      if (position) entityPositions.set(key, position)
      if (tweenActive(tween, now)) animating = true
    }

    return {
      player: sampleTween(this.playerTween, now),
      camera: sampleTween(this.cameraTween, now),
      entityPositions,
      animating,
      discontinuity: this.discontinuity,
      sourceFrame: Math.max(0, this.sourceFrame),
      sourceCapturedAtUnixMS: this.sourceCapturedAtUnixMS
    }
  }

  private tweenDuration(arrivalGap: number): number {
    if (arrivalGap <= 0) return this.minTweenMs
    return clamp(arrivalGap * 0.8, this.minTweenMs, this.maxTweenMs)
  }

  private makeTween(from: RenderPosition, to: RenderPosition, startedAt: number, durationMs: number): Tween {
    return {
      from: point(from),
      to: point(to),
      startedAt,
      durationMs: samePoint(from, to) ? 0 : durationMs
    }
  }

  private snap(
    state: RenderState,
    player: RenderPosition,
    receivedAt: number,
    discontinuity: AnimationDiscontinuity
  ): void {
    this.playerTween = this.makeTween(player, player, receivedAt, 0)
    this.cameraTween = this.makeTween(player, player, receivedAt, 0)
    this.entityTweens.clear()
    for (const [index, actor] of (state.entities || []).entries()) {
      const target = authoritativePosition(actor)
      this.entityTweens.set(actorKey(actor, index), this.makeTween(target, target, receivedAt, 0))
    }
    this.mapID = state.map?.id || ''
    this.sourceFrame = Number(state.clock.frame || 0)
    this.sourceCapturedAtUnixMS = state.clock.captured_at_unix_ms
    this.lastReceivedAt = receivedAt
    this.discontinuity = discontinuity
  }

  private retargetEntities(actors: RenderActor[], receivedAt: number, durationMs: number): void {
    const current = new Map(this.entityTweens)
    const next = new Map<string, Tween>()

    actors.forEach((actor, index) => {
      const key = actorKey(actor, index)
      const target = authoritativePosition(actor)
      const existing = current.get(key)
      const from = sampleTween(existing || null, receivedAt) || target
      const teleported = distance(from, target) > this.entityTeleportDistanceTiles
      next.set(key, this.makeTween(teleported ? target : from, target, receivedAt, teleported ? 0 : durationMs))
    })
    this.entityTweens = next
  }
}

export function presentationEntityKey(actor: RenderActor, index: number): string {
  return actorKey(actor, index)
}
