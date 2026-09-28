import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const output = path.resolve(here, '../../cmd/pokeui/ui/vue/spectator')
const safeTown = path.join(output, 'theme-assets/kenney-tiny-town/tilemap.png')
const safeDungeon = path.join(output, 'theme-assets/kenney-tiny-dungeon/tilemap.png')
const restricted = path.join(output, 'theme-assets/pokegold-gen2')

for (const asset of [safeTown, safeDungeon]) {
  if (!fs.existsSync(asset)) throw new Error(`public spectator build is missing safe theme asset: ${asset}`)
}
if (fs.existsSync(restricted)) {
  throw new Error(`public spectator build contains local-only theme assets: ${restricted}`)
}

console.log('spectator theme distribution: public-safe assets only')
