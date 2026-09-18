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

  const paths = []
  for (const name of [...mapNames].sort()) paths.push(`maps/${name}.blk`)
  for (const stem of [...stems].sort()) {
    paths.push(`gfx/blocksets/${stem}.bst`)
    paths.push(`gfx/tilesets/${stem}.png`)
  }
  return { mapNames, stems, paths }
}

function outputPath(relativePath) {
  if (relativePath.startsWith('maps/')) return path.join(outRoot, relativePath)
  if (relativePath.startsWith('gfx/blocksets/')) {
    return path.join(outRoot, 'blocksets', path.basename(relativePath))
  }
  if (relativePath.startsWith('gfx/tilesets/')) {
    return path.join(outRoot, 'tilesets', path.basename(relativePath))
  }
  throw new Error(`unsupported render asset ${relativePath}`)
}

function markerMatches(expected) {
  try {
    const marker = JSON.parse(fs.readFileSync(markerPath, 'utf8'))
    if (marker.commit !== POKERED_RENDER_COMMIT) return false
    return expected.paths.every((relativePath) => fs.existsSync(outputPath(relativePath)))
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

function extractSelectedTarEntries(tar, wanted) {
  const extracted = new Set()
  let offset = 0

  while (offset + 512 <= tar.length) {
    const header = tar.subarray(offset, offset + 512)
    if (header.every((byte) => byte === 0)) break

    const name = readTarString(header.subarray(0, 100))
    const size = readTarOctal(header.subarray(124, 136))
    const dataStart = offset + 512
    const dataEnd = dataStart + size

    const slash = name.indexOf('/')
    const relative = slash >= 0 ? name.slice(slash + 1) : name
    if (wanted.has(relative)) {
      const destination = outputPath(relative)
      fs.mkdirSync(path.dirname(destination), { recursive: true })
      fs.writeFileSync(destination, tar.subarray(dataStart, dataEnd))
      extracted.add(relative)
    }

    offset = dataStart + Math.ceil(size / 512) * 512
  }

  return extracted
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
  const extracted = extractSelectedTarEntries(tar, new Set(expected.paths))

  const missing = expected.paths.filter((relativePath) => !extracted.has(relativePath))
  if (missing.length) {
    throw new Error(`pret/pokered archive is missing ${missing.length} expected render asset(s): ${missing.slice(0, 8).join(', ')}`)
  }

  const marker = {
    source: 'pret/pokered',
    commit: POKERED_RENDER_COMMIT,
    maps: expected.mapNames.size,
    tilesets: [...expected.stems].sort(),
    note: 'Generated at build time; original game graphics are not committed to PokePilot.'
  }
  fs.writeFileSync(markerPath, JSON.stringify(marker, null, 2) + '\n')
  console.log(`gen1 render assets: mirrored ${expected.mapNames.size} maps and ${expected.stems.size} tilesets`)
}

await main()
