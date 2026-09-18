import fs from 'node:fs'
import path from 'node:path'
import { gunzipSync } from 'node:zlib'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(here, '../..')
const headersDir = path.join(repoRoot, 'pokered/data/maps/headers')
const outRoot = path.join(repoRoot, 'web/public/gen1/red')
const markerPath = path.join(outRoot, 'SOURCE.json')

export const POKERED_RENDER_COMMIT = '0cd19d3'
const archiveURL = `https://codeload.github.com/pret/pokered/tar.gz/${POKERED_RENDER_COMMIT}`

const TILESET_ASSET_STEMS = {
  OVERWORLD: 'overworld',
  REDS_HOUSE_1: 'reds_house',
  MART: 'pokecenter',
  FOREST: 'forest',
  REDS_HOUSE_2: 'reds_house',
  DOJO: 'gym',
  POKECENTER: 'pokecenter',
  GYM: 'gym',
  HOUSE: 'house',
  FOREST_GATE: 'gate',
  MUSEUM: 'gate',
  UNDERGROUND: 'underground',
  GATE: 'gate',
  SHIP: 'ship',
  SHIP_PORT: 'ship_port',
  CEMETERY: 'cemetery',
  INTERIOR: 'interior',
  CAVERN: 'cavern',
  LOBBY: 'lobby',
  MANSION: 'mansion',
  LAB: 'lab',
  CLUB: 'club',
  FACILITY: 'facility',
  PLATEAU: 'plateau'
}

function expectedAssets() {
  const mapNames = new Set()
  const stems = new Set()

  for (const file of fs.readdirSync(headersDir).filter((name) => name.endsWith('.asm'))) {
    const source = fs.readFileSync(path.join(headersDir, file), 'utf8')
    const header = source.match(/map_header\s+([^,\s]+),\s*[A-Z0-9_]+,\s*([A-Z0-9_]+)/)
    if (!header) continue
    mapNames.add(header[1])
    const stem = TILESET_ASSET_STEMS[header[2]]
    if (!stem) throw new Error(`${file}: no render asset mapping for tileset ${header[2]}`)
    stems.add(stem)
  }

  return { mapNames, stems }
}

function outputPath(relativePath) {
  if (relativePath.startsWith('maps/')) return path.join(outRoot, relativePath)
  if (relativePath.startsWith('gfx/blocksets/')) {
    return path.join(outRoot, 'blocksets', path.basename(relativePath))
  }
  if (relativePath.startsWith('gfx/tilesets/')) {
    return path.join(outRoot, 'tilesets', path.basename(relativePath))
  }
  if (relativePath.startsWith('gfx/sprites/')) {
    return path.join(outRoot, 'sprites', path.basename(relativePath))
  }
  throw new Error(`unsupported render asset ${relativePath}`)
}

function markerMatches(expected) {
  try {
    const marker = JSON.parse(fs.readFileSync(markerPath, 'utf8'))
    if (marker.commit !== POKERED_RENDER_COMMIT) return false
    for (const name of expected.mapNames) {
      if (!fs.existsSync(path.join(outRoot, 'maps', `${name}.blk`))) return false
    }
    for (const stem of expected.stems) {
      if (!fs.existsSync(path.join(outRoot, 'blocksets', `${stem}.bst`))) return false
      if (!fs.existsSync(path.join(outRoot, 'tilesets', `${stem}.png`))) return false
    }
    const spriteDir = path.join(outRoot, 'sprites')
    if (!fs.existsSync(spriteDir) || fs.readdirSync(spriteDir).filter((name) => name.endsWith('.png')).length < 50) return false
    return true
  } catch {
    return false
  }
}

function readTarString(buffer) {
  const end = buffer.indexOf(0)
  return buffer.subarray(0, end === -1 ? buffer.length : end).toString('utf8')
}

function readTarOctal(buffer) {
  const raw = readTarString(buffer).trim()
  return raw ? Number.parseInt(raw, 8) : 0
}

function extractRenderEntries(tar, stems) {
  let mapsAsm = ''
  let offset = 0

  while (offset + 512 <= tar.length) {
    const header = tar.subarray(offset, offset + 512)
    if (header.every((byte) => byte === 0)) break

    const name = readTarString(header.subarray(0, 100))
    const size = readTarOctal(header.subarray(124, 136))
    const dataStart = offset + 512
    const dataEnd = dataStart + size
    const data = tar.subarray(dataStart, dataEnd)

    const slash = name.indexOf('/')
    const relative = slash >= 0 ? name.slice(slash + 1) : name

    if (relative === 'maps.asm') {
      mapsAsm = data.toString('utf8')
    } else if (/^maps\/[^/]+\.blk$/.test(relative)) {
      const destination = outputPath(relative)
      fs.mkdirSync(path.dirname(destination), { recursive: true })
      fs.writeFileSync(destination, data)
    } else if (/^gfx\/sprites\/[^/]+\.png$/.test(relative)) {
      const destination = outputPath(relative)
      fs.mkdirSync(path.dirname(destination), { recursive: true })
      fs.writeFileSync(destination, data)
    } else {
      const blockset = relative.match(/^gfx\/blocksets\/([^/]+)\.bst$/)
      const tileset = relative.match(/^gfx\/tilesets\/([^/]+)\.png$/)
      const stem = blockset?.[1] || tileset?.[1]
      if (stem && stems.has(stem)) {
        const destination = outputPath(relative)
        fs.mkdirSync(path.dirname(destination), { recursive: true })
        fs.writeFileSync(destination, data)
      }
    }

    offset = dataStart + Math.ceil(size / 512) * 512
  }

  if (!mapsAsm) throw new Error('pret/pokered archive did not contain maps.asm')
  return mapsAsm
}

function resolveBlockAliases(mapsAsm) {
  const aliases = new Map()
  let pending = []

  for (const rawLine of mapsAsm.split(/\r?\n/)) {
    const line = rawLine.trim()
    const label = line.match(/^([A-Za-z0-9]+)_Blocks:(?:\s+INCBIN\s+"maps\/([^"]+\.blk)")?/)
    if (label) {
      pending.push(label[1])
      if (label[2]) {
        for (const name of pending) aliases.set(name, label[2])
        pending = []
      }
      continue
    }

    const incbin = line.match(/^INCBIN\s+"maps\/([^"]+\.blk)"/)
    if (incbin && pending.length) {
      for (const name of pending) aliases.set(name, incbin[1])
      pending = []
    }
  }

  return aliases
}

function materializeMapAliases(mapNames, mapsAsm) {
  const aliases = resolveBlockAliases(mapsAsm)
  const missing = []

  for (const name of mapNames) {
    const destination = path.join(outRoot, 'maps', `${name}.blk`)
    if (fs.existsSync(destination)) continue

    const sourceName = aliases.get(name)
    const source = sourceName ? path.join(outRoot, 'maps', sourceName) : ''
    if (!sourceName || !fs.existsSync(source)) {
      missing.push(name)
      continue
    }
    fs.copyFileSync(source, destination)
  }

  return missing
}

async function main() {
  if (process.env.POKEPILOT_SKIP_GEN1_TEXTURES === '1') {
    console.warn('gen1 render assets: skipped by POKEPILOT_SKIP_GEN1_TEXTURES=1')
    return
  }

  const expected = expectedAssets()
  if (markerMatches(expected)) {
    console.log(`gen1 render assets: cached at pret/pokered ${POKERED_RENDER_COMMIT}`)
    return
  }

  console.log(`gen1 render assets: fetching pret/pokered ${POKERED_RENDER_COMMIT}`)
  const response = await fetch(archiveURL, { redirect: 'follow' })
  if (!response.ok) throw new Error(`failed to download ${archiveURL}: HTTP ${response.status}`)
  const compressed = Buffer.from(await response.arrayBuffer())
  const tar = gunzipSync(compressed)

  fs.rmSync(outRoot, { recursive: true, force: true })
  fs.mkdirSync(outRoot, { recursive: true })
  const mapsAsm = extractRenderEntries(tar, expected.stems)
  const missingMaps = materializeMapAliases(expected.mapNames, mapsAsm)

  const missingAssets = []
  for (const stem of expected.stems) {
    if (!fs.existsSync(path.join(outRoot, 'blocksets', `${stem}.bst`))) missingAssets.push(`blocksets/${stem}.bst`)
    if (!fs.existsSync(path.join(outRoot, 'tilesets', `${stem}.png`))) missingAssets.push(`tilesets/${stem}.png`)
  }
  if (missingMaps.length || missingAssets.length) {
    throw new Error(
      `pret/pokered render sync incomplete: ${missingMaps.length} map(s), ${missingAssets.length} tileset asset(s) missing; ` +
      `examples: ${[...missingMaps.slice(0, 5), ...missingAssets.slice(0, 5)].join(', ')}`
    )
  }

  const marker = {
    source: 'pret/pokered',
    commit: POKERED_RENDER_COMMIT,
    maps: expected.mapNames.size,
    tilesets: [...expected.stems].sort(),
    sprites: fs.readdirSync(path.join(outRoot, 'sprites')).filter((name) => name.endsWith('.png')).length,
    note: 'Generated at build time; original game graphics are not committed to PokePilot.'
  }
  fs.writeFileSync(markerPath, JSON.stringify(marker, null, 2) + '\n')
  const spriteCount = fs.readdirSync(path.join(outRoot, 'sprites')).filter((name) => name.endsWith('.png')).length
  console.log(`gen1 render assets: mirrored ${expected.mapNames.size} maps, ${expected.stems.size} tilesets, and ${spriteCount} sprites`)
}

await main()
