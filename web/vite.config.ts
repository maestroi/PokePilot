import { rmSync } from 'node:fs'
import { resolve } from 'node:path'
import { defineConfig, loadEnv, type Plugin } from 'vite'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'

function publicThemeGate(target: 'spectator' | 'operator'): Plugin {
  return {
    name: 'pokepilot-public-theme-gate',
    closeBundle() {
      if (target !== 'spectator') return
      const output = resolve(process.cwd(), '../cmd/pokeui/ui/vue/spectator')
      rmSync(resolve(output, 'theme-assets/pokegold-gen2'), { recursive: true, force: true })
    }
  }
}

export default defineConfig(({ mode, command }) => {
  const target = mode === 'spectator' ? 'spectator' : 'operator'
  const env = loadEnv(mode, '.', '')
  const devBackend = env.POKEPILOT_DEV_BACKEND || (target === 'spectator'
    ? 'http://localhost:18081'
    : 'http://localhost:18080')

  return {
    base: '/',
    plugins: [vue(), tailwindcss(), publicThemeGate(target)],
    server: command === 'serve' ? {
      proxy: {
        '/v1': devBackend,
        '/frame': devBackend,
        '/render-state': devBackend,
        '/maps': devBackend
      }
    } : undefined,
    build: {
      outDir: `../cmd/pokeui/ui/vue/${target}`,
      emptyOutDir: true,
      rollupOptions: {
        input: target === 'spectator'
          ? { spectator: 'spectator.html', world: 'world.html', replays: 'replays.html' }
          : 'operator.html'
      }
    }
  }
})
