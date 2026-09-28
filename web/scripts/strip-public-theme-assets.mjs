import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const output = path.resolve(here, '../../cmd/pokeui/ui/vue/spectator')
const restricted = path.join(output, 'theme-assets/pokegold-gen2')

fs.rmSync(restricted, { recursive: true, force: true })
console.log('spectator theme distribution: removed local-only Gold / Silver assets')
