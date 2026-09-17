import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(here, '../..')
const constantsPath = path.join(repoRoot, 'pokered/constants/map_constants.asm')
const headersDir = path.join(repoRoot, 'pokered/data/maps/headers')
const objectsDir = path.join(repoRoot, 'pokered/data/maps/objects')
const outputPath = path.join(repoRoot, 'web/src/shared/redWorldManifest.generated.ts')

const constants = fs.readFileSync(constantsPath, 'utf8')
const mapsByName = new Map()
const mapsByID = new Map()
const mapsBySourceName = new Map()

function humanizeSymbol(value) {
  return String(value || '')
    .replace(/^(SPRITE_|OPP_|TEXT_|ITEM_)/, '')
    .toLowerCase()
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

for (const line of constants.split(/\r?\n/)) {
  const match = line.match(/map_const\s+([A-Z0-9_]+),\s*(\d+),\s*(\d+)\s*;\s*\$([0-9A-Fa-f]{2})/)
  if (!match) continue
  const [, name, widthBlocks, heightBlocks, hexID] = match
  const map = {
    id: Number.parseInt(hexID, 16),
    name,
    width: Number(widthBlocks) * 2,
    height: Number(heightBlocks) * 2,
    connections: [],
    pois: []
  }
  mapsByName.set(name, map)
  mapsByID.set(map.id, map)
}

for (const file of fs.readdirSync(headersDir).filter((name) => name.endsWith('.asm'))) {
  const source = fs.readFileSync(path.join(headersDir, file), 'utf8')
  const header = source.match(/map_header\s+([^,\s]+),\s*([A-Z0-9_]+),/)
  if (!header) continue
  const map = mapsByName.get(header[2])
  if (!map) continue
  map.sourceName = header[1]
  mapsBySourceName.set(header[1], map)

  for (const line of source.split(/\r?\n/)) {
    const connection = line.match(/connection\s+(north|south|west|east),\s*[^,]+,\s*([A-Z0-9_]+),\s*(-?\d+)/)
    if (!connection) continue
    const [, direction, destinationName, offsetText] = connection
    const destination = mapsByName.get(destinationName)
    if (!destination) {
      throw new Error(`${file}: unknown connection destination ${destinationName}`)
    }
    map.connections.push({
      direction,
      to: destination.id,
      offsetBlocks: Number(offsetText)
    })
  }
}


for (const file of fs.readdirSync(objectsDir).filter((name) => name.endsWith('.asm'))) {
  const sourceName = path.basename(file, '.asm')
  const map = mapsBySourceName.get(sourceName)
  if (!map) continue

  const source = fs.readFileSync(path.join(objectsDir, file), 'utf8')
  for (const rawLine of source.split(/\r?\n/)) {
    const line = rawLine.split(';', 1)[0].trim()
    if (!line) continue

    const bg = line.match(/^bg_event\s+(-?\d+)\s*,\s*(-?\d+)\s*,\s*([A-Z0-9_]+)/)
    if (bg) {
      const x = Number(bg[1])
      const y = Number(bg[2])
      if (x >= 0 && y >= 0 && x < map.width && y < map.height) {
        map.pois.push({ x, y, kind: 'sign', label: 'Sign' })
      }
      continue
    }

    if (!line.startsWith('object_event ')) continue
    const args = line.slice('object_event '.length).split(',').map((part) => part.trim())
    if (args.length < 6) continue
    const x = Number(args[0])
    const y = Number(args[1])
    if (!Number.isFinite(x) || !Number.isFinite(y) || x < 0 || y < 0 || x >= map.width || y >= map.height) continue

    const sprite = args[2] || ''
    let kind = 'npc'
    let label = humanizeSymbol(sprite) || 'NPC'
    if (args[6]?.startsWith('OPP_')) {
      kind = 'trainer'
      label = humanizeSymbol(args[6]) || 'Trainer'
    } else if (args.length >= 7) {
      kind = 'item'
      label = humanizeSymbol(args[6]) || 'Item'
    }
    map.pois.push({ x, y, kind, label })
  }
}

const outdoor = [...mapsByID.values()]
  .filter((map) => map.id <= 0x24 && map.width > 0 && map.height > 0)
  .sort((a, b) => a.id - b.id)

const positions = new Map()
const conflicts = []
let nextComponentX = 0

function placeFrom(source, destination, connection, origin) {
  const delta = connection.offsetBlocks * 2
  switch (connection.direction) {
    case 'north':
      return { x: origin.x + delta, y: origin.y - destination.height }
    case 'south':
      return { x: origin.x + delta, y: origin.y + source.height }
    case 'west':
      return { x: origin.x - destination.width, y: origin.y + delta }
    case 'east':
      return { x: origin.x + source.width, y: origin.y + delta }
    default:
      throw new Error(`unknown direction ${connection.direction}`)
  }
}

for (const anchor of outdoor) {
  if (positions.has(anchor.id)) continue
  positions.set(anchor.id, { x: nextComponentX, y: 0 })
  const queue = [anchor]
  const componentIDs = []

  while (queue.length) {
    const source = queue.shift()
    componentIDs.push(source.id)
    const origin = positions.get(source.id)
    for (const connection of source.connections) {
      const destination = mapsByID.get(connection.to)
      if (!destination || destination.id > 0x24 || destination.width <= 0 || destination.height <= 0) continue
      const candidate = placeFrom(source, destination, connection, origin)
      const existing = positions.get(destination.id)
      if (!existing) {
        positions.set(destination.id, candidate)
        queue.push(destination)
        continue
      }
      if (existing.x !== candidate.x || existing.y !== candidate.y) {
        conflicts.push(`${source.name} ${connection.direction} ${destination.name}: (${existing.x},${existing.y}) != (${candidate.x},${candidate.y})`)
      }
    }
  }

  const componentMaps = componentIDs.map((id) => {
    const map = mapsByID.get(id)
    const position = positions.get(id)
    return { ...map, ...position }
  })
  const maxX = Math.max(...componentMaps.map((map) => map.x + map.width))
  nextComponentX = maxX + 80
}

const positioned = outdoor.map((map) => ({ ...map, ...positions.get(map.id) }))
const minX = Math.min(...positioned.map((map) => map.x))
const minY = Math.min(...positioned.map((map) => map.y))
for (const [id, position] of positions) {
  positions.set(id, { x: position.x - minX, y: position.y - minY })
}

if (conflicts.length) {
  console.warn(`world manifest: ${conflicts.length} placement conflict(s); keeping first placement`)
  for (const conflict of conflicts.slice(0, 8)) console.warn(`  ${conflict}`)
}

const manifest = [...mapsByID.values()]
  .sort((a, b) => a.id - b.id)
  .map((map) => {
    const position = positions.get(map.id)
    return {
      id: map.id,
      name: map.name,
      width: map.width,
      height: map.height,
      ...(position ? position : {}),
      connections: map.connections,
      pois: map.pois
    }
  })

const banner = `// Code generated by web/scripts/generate-red-world-manifest.mjs; DO NOT EDIT.\n// Source: vendored pokered map constants + map headers. No ROM bytes or graphics are included.\n\n`
const body = `export const RED_WORLD_MANIFEST = ${JSON.stringify(manifest, null, 2)} as const\n`
fs.writeFileSync(outputPath, banner + body)
console.log(`generated ${path.relative(repoRoot, outputPath)} (${manifest.length} maps, ${outdoor.length} atlas maps)`)
