import pokegoldManifest from './themes/pokegold-gen2.json' with { type: 'json' }
import kenneyManifest from './themes/kenney-tiny-town.json' with { type: 'json' }

export const RENDER_THEME_SCHEMA_VERSION = 1
export const DEFAULT_RENDER_THEME_ID = 'pokegold-gen2'

export const REQUIRED_THEME_TILES = ['unknown', 'path', 'wall'] as const
export const OPTIONAL_THEME_TILES = ['floor', 'grass', 'water', 'tree', 'ledge', 'door', 'warp', 'sign'] as const

export type ThemeTilePattern =
  | 'plain'
  | 'unknown'
  | 'path'
  | 'floor'
  | 'grass'
  | 'water'
  | 'tree'
  | 'ledge'
  | 'wall'
  | 'door'
  | 'warp'
  | 'sign'

export interface ThemeTileStyle {
  fill: string
  detail?: string
  accent?: string
  pattern?: ThemeTilePattern
}

export interface ThemeActorStyle {
  fill: string
  stroke: string
}

export interface RenderThemeManifest {
  schemaVersion: number
  id: string
  name: string
  version: number
  description?: string
  tileSize: number
  tiles: Record<string, ThemeTileStyle>
  objects?: Record<string, ThemeTileStyle>
  actors?: Partial<Record<'player' | 'npc' | 'trainer' | 'item' | 'object', ThemeActorStyle>>
  animation?: {
    waterPeriodMs?: number
    redrawIntervalMs?: number
  }
  effects?: {
    background?: string
    vignette?: string
    grid?: string
    shadow?: string
  }
  ui?: {
    accent?: string
    panel?: string
    text?: string
  }
  assets?: {
    tiles?: Record<string, string>
    characters?: Record<string, string>
    objects?: Record<string, string>
    effects?: Record<string, string>
    ui?: Record<string, string>
    battle?: Record<string, string>
  }
  battle?: Record<string, string>
}

export interface ResolvedRenderTheme extends RenderThemeManifest {
  objects: Record<string, ThemeTileStyle>
  actors: Record<'player' | 'npc' | 'trainer' | 'item' | 'object', ThemeActorStyle>
  animation: {
    waterPeriodMs: number
    redrawIntervalMs: number
  }
  effects: {
    background: string
    vignette: string
    grid: string
    shadow: string
  }
  ui: {
    accent: string
    panel: string
    text: string
  }
  assets: {
    tiles: Record<string, string>
    characters: Record<string, string>
    objects: Record<string, string>
    effects: Record<string, string>
    ui: Record<string, string>
    battle: Record<string, string>
  }
  battle: Record<string, string>
}

export interface ThemeValidation {
  ok: boolean
  errors: string[]
  warnings: string[]
}

export interface ThemeInstallResult {
  installed: boolean
  diagnostics: string[]
}

export interface ThemeResolution {
  theme: ResolvedRenderTheme
  diagnostics: string[]
}

function record(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function nonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length > 0
}

function validPaint(value: unknown): boolean {
  return nonEmptyString(value) && value.length <= 96
}

function validateTileStyle(value: unknown, path: string, errors: string[]): void {
  if (!record(value)) {
    errors.push(`${path} must be an object`)
    return
  }
  if (!validPaint(value.fill)) errors.push(`${path}.fill must be a non-empty paint value`)
  if (value.detail !== undefined && !validPaint(value.detail)) errors.push(`${path}.detail must be a paint value`)
  if (value.accent !== undefined && !validPaint(value.accent)) errors.push(`${path}.accent must be a paint value`)
  if (value.pattern !== undefined && !nonEmptyString(value.pattern)) errors.push(`${path}.pattern must be a string`)
}

function validateStringMap(value: unknown, path: string, errors: string[]): void {
  if (value === undefined) return
  if (!record(value)) {
    errors.push(`${path} must be an object`)
    return
  }
  for (const [key, entry] of Object.entries(value)) {
    if (!nonEmptyString(key) || !nonEmptyString(entry) || entry.length > 512) {
      errors.push(`${path}.${key || '<empty>'} must be a non-empty asset reference`)
    }
  }
}

export function validateThemePack(input: unknown): ThemeValidation {
  const errors: string[] = []
  const warnings: string[] = []

  if (!record(input)) {
    return { ok: false, errors: ['theme pack must be an object'], warnings }
  }
  if (input.schemaVersion !== RENDER_THEME_SCHEMA_VERSION) {
    errors.push(`schemaVersion ${String(input.schemaVersion)} is incompatible; expected ${RENDER_THEME_SCHEMA_VERSION}`)
  }
  if (!nonEmptyString(input.id) || !/^[a-z0-9][a-z0-9-]*$/.test(input.id)) {
    errors.push('id must use lowercase letters, numbers, and hyphens')
  }
  if (!nonEmptyString(input.name)) errors.push('name is required')
  if (typeof input.version !== 'number' || !Number.isInteger(input.version) || input.version < 1) {
    errors.push('version must be a positive integer')
  }
  if (typeof input.tileSize !== 'number' || !Number.isFinite(input.tileSize) || input.tileSize < 16 || input.tileSize > 64) {
    errors.push('tileSize must be between 16 and 64')
  }

  if (!record(input.tiles)) {
    errors.push('tiles must be an object')
  } else {
    for (const required of REQUIRED_THEME_TILES) {
      if (!(required in input.tiles)) errors.push(`tiles.${required} is required`)
    }
    for (const [kind, style] of Object.entries(input.tiles)) validateTileStyle(style, `tiles.${kind}`, errors)
    for (const optional of OPTIONAL_THEME_TILES) {
      if (!(optional in input.tiles)) warnings.push(`tiles.${optional} missing; default theme fallback will be used`)
    }
  }

  if (input.objects !== undefined) {
    if (!record(input.objects)) errors.push('objects must be an object')
    else for (const [kind, style] of Object.entries(input.objects)) validateTileStyle(style, `objects.${kind}`, errors)
  }

  if (input.actors !== undefined) {
    if (!record(input.actors)) {
      errors.push('actors must be an object')
    } else {
      for (const [kind, style] of Object.entries(input.actors)) {
        if (!record(style) || !validPaint(style.fill) || !validPaint(style.stroke)) {
          errors.push(`actors.${kind} must define fill and stroke`)
        }
      }
    }
  }

  if (input.animation !== undefined) {
    if (!record(input.animation)) {
      errors.push('animation must be an object')
    } else {
      for (const key of ['waterPeriodMs', 'redrawIntervalMs']) {
        const value = input.animation[key]
        if (value !== undefined && (typeof value !== 'number' || !Number.isFinite(value) || value < 16 || value > 60_000)) {
          errors.push(`animation.${key} must be between 16 and 60000 milliseconds`)
        }
      }
    }
  }

  if (input.assets !== undefined) {
    if (!record(input.assets)) {
      errors.push('assets must be an object')
    } else {
      for (const section of ['tiles', 'characters', 'objects', 'effects', 'ui', 'battle']) {
        validateStringMap(input.assets[section], `assets.${section}`, errors)
      }
    }
  }
  validateStringMap(input.battle, 'battle', errors)

  return { ok: errors.length === 0, errors, warnings }
}

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function actorDefaults(): Record<'player' | 'npc' | 'trainer' | 'item' | 'object', ThemeActorStyle> {
  return {
    player: { fill: '#f4f7ff', stroke: '#e43c4f' },
    npc: { fill: '#f2d071', stroke: '#18252a' },
    trainer: { fill: '#f6a65d', stroke: '#18252a' },
    item: { fill: '#d9c4ff', stroke: '#3d315a' },
    object: { fill: '#b8c5ca', stroke: '#27343a' }
  }
}

function materializeBase(manifest: RenderThemeManifest): ResolvedRenderTheme {
  return {
    ...clone(manifest),
    tiles: clone(manifest.tiles),
    objects: clone(manifest.objects || {}),
    actors: { ...actorDefaults(), ...(manifest.actors || {}) },
    animation: {
      waterPeriodMs: manifest.animation?.waterPeriodMs ?? 700,
      redrawIntervalMs: manifest.animation?.redrawIntervalMs ?? 100
    },
    effects: {
      background: manifest.effects?.background || '#142228',
      vignette: manifest.effects?.vignette || 'rgba(0,0,0,.28)',
      grid: manifest.effects?.grid || 'rgba(255,255,255,.045)',
      shadow: manifest.effects?.shadow || 'rgba(0,0,0,.22)'
    },
    ui: {
      accent: manifest.ui?.accent || '#67e8f9',
      panel: manifest.ui?.panel || 'rgba(2,6,23,.72)',
      text: manifest.ui?.text || '#cffafe'
    },
    assets: {
      tiles: clone(manifest.assets?.tiles || {}),
      characters: clone(manifest.assets?.characters || {}),
      objects: clone(manifest.assets?.objects || {}),
      effects: clone(manifest.assets?.effects || {}),
      ui: clone(manifest.assets?.ui || {}),
      battle: clone(manifest.assets?.battle || {})
    },
    battle: clone(manifest.battle || {})
  }
}

function mergeTheme(base: ResolvedRenderTheme, manifest: RenderThemeManifest): ResolvedRenderTheme {
  return {
    ...clone(manifest),
    tiles: { ...clone(base.tiles), ...clone(manifest.tiles) },
    objects: { ...clone(base.objects), ...clone(manifest.objects || {}) },
    actors: { ...clone(base.actors), ...(manifest.actors || {}) },
    animation: {
      waterPeriodMs: manifest.animation?.waterPeriodMs ?? base.animation.waterPeriodMs,
      redrawIntervalMs: manifest.animation?.redrawIntervalMs ?? base.animation.redrawIntervalMs
    },
    effects: { ...clone(base.effects), ...(manifest.effects || {}) },
    ui: { ...clone(base.ui), ...(manifest.ui || {}) },
    assets: {
      tiles: { ...clone(base.assets.tiles), ...(manifest.assets?.tiles || {}) },
      characters: { ...clone(base.assets.characters), ...(manifest.assets?.characters || {}) },
      objects: { ...clone(base.assets.objects), ...(manifest.assets?.objects || {}) },
      effects: { ...clone(base.assets.effects), ...(manifest.assets?.effects || {}) },
      ui: { ...clone(base.assets.ui), ...(manifest.assets?.ui || {}) },
      battle: { ...clone(base.assets.battle), ...(manifest.assets?.battle || {}) }
    },
    battle: { ...clone(base.battle), ...(manifest.battle || {}) }
  }
}

export class RenderThemeRegistry {
  private readonly manifests = new Map<string, RenderThemeManifest>()
  private defaultID = ''

  install(input: unknown, makeDefault = false): ThemeInstallResult {
    const validation = validateThemePack(input)
    const diagnostics = [...validation.errors, ...validation.warnings]
    if (!validation.ok) return { installed: false, diagnostics }

    const manifest = clone(input as RenderThemeManifest)
    this.manifests.set(manifest.id, manifest)
    if (!this.defaultID || makeDefault) this.defaultID = manifest.id
    return { installed: true, diagnostics }
  }

  list(): RenderThemeManifest[] {
    return [...this.manifests.values()].map(clone)
  }

  resolve(id?: string | null): ThemeResolution {
    const fallbackManifest = this.manifests.get(this.defaultID)
    if (!fallbackManifest) throw new Error('render theme registry has no default theme')
    const fallback = materializeBase(fallbackManifest)

    if (!id) return { theme: fallback, diagnostics: [] }
    const selected = this.manifests.get(id)
    if (!selected) {
      return {
        theme: fallback,
        diagnostics: [`Theme "${id}" is unavailable; using ${fallback.name}.`]
      }
    }
    if (selected.id === fallback.id) return { theme: fallback, diagnostics: [] }
    return { theme: mergeTheme(fallback, selected), diagnostics: [] }
  }
}

export const renderThemeRegistry = new RenderThemeRegistry()

function installBundled(input: unknown, makeDefault = false): void {
  const result = renderThemeRegistry.install(input, makeDefault)
  if (!result.installed) throw new Error(`invalid bundled render theme: ${result.diagnostics.join('; ')}`)
}

installBundled(pokegoldManifest, true)
installBundled(kenneyManifest)

export function renderThemeOptions(): RenderThemeManifest[] {
  return renderThemeRegistry.list()
}

export function resolveRenderTheme(id?: string | null): ThemeResolution {
  return renderThemeRegistry.resolve(id || DEFAULT_RENDER_THEME_ID)
}

export function tileStyle(theme: ResolvedRenderTheme, kind: string, objectLayer = false): ThemeTileStyle {
  if (objectLayer && theme.objects[kind]) return theme.objects[kind]
  return theme.tiles[kind] || theme.tiles.unknown
}

export function actorStyle(theme: ResolvedRenderTheme, kind: string, player = false): ThemeActorStyle {
  if (player) return theme.actors.player
  if (kind === 'trainer' || kind === 'item' || kind === 'object') return theme.actors[kind]
  return theme.actors.npc
}

export function characterAsset(theme: ResolvedRenderTheme, appearance: string | undefined, player = false): string {
  const key = player ? 'player' : (appearance || '').toLowerCase()
  return theme.assets.characters[key] || ''
}
