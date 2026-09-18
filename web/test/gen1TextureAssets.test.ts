import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { test } from 'node:test'
import { fileURLToPath } from 'node:url'
import { expandGen1Blocks } from '../src/shared/gen1TextureCodec.ts'

const here = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(here, '../..')
const publicRoot = path.join(repoRoot, 'web/public/gen1/red')

function pngDimensions(buffer: Buffer): { width: number, height: number } {
  assert.equal(buffer.subarray(1, 4).toString('ascii'), 'PNG')
  assert.equal(buffer.subarray(12, 16).toString('ascii'), 'IHDR')
  return {
    width: buffer.readUInt32BE(16),
    height: buffer.readUInt32BE(20)
  }
}

function mapBlockDimensions(name: string): { width: number, height: number } {
  const constants = fs.readFileSync(path.join(repoRoot, 'pokered/constants/map_constants.asm'), 'utf8')
  const match = constants.match(new RegExp(`map_const\\s+${name},\\s*(\\d+),\\s*(\\d+)`))
  assert.ok(match, `missing map_const for ${name}`)
  return { width: Number(match[1]), height: Number(match[2]) }
}

test('Route 1 overworld assets can represent every referenced tile', () => {
  const mapPath = path.join(publicRoot, 'maps/Route1.blk')
  if (!fs.existsSync(mapPath)) return

  const mapBlocks = new Uint8Array(fs.readFileSync(mapPath))
  const blockset = new Uint8Array(fs.readFileSync(path.join(publicRoot, 'blocksets/overworld.bst')))
  const png = fs.readFileSync(path.join(publicRoot, 'tilesets/overworld.png'))
  const { width, height } = pngDimensions(png)
  const dims = mapBlockDimensions('ROUTE_1')

  assert.equal(mapBlocks.length, dims.width * dims.height)
  const tiles = expandGen1Blocks(mapBlocks, dims.width, dims.height, blockset)
  const maxTile = Math.max(...tiles)
  const capacity = Math.floor(width / 8) * Math.floor(height / 8)

  console.log(JSON.stringify({
    route1Blocks: mapBlocks.length,
    blocksetBytes: blockset.length,
    png: `${width}x${height}`,
    tileCapacity: capacity,
    maxTile
  }))

  assert.ok(maxTile < capacity, `Route 1 needs tile 0x${maxTile.toString(16)} but overworld PNG only exposes ${capacity} 8x8 tiles`)
})
