import { defineConfig, loadEnv } from 'vite'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig(({ mode, command }) => {
  const target = mode === 'spectator' ? 'spectator' : 'operator'
  const env = loadEnv(mode, '.', '')
  const devBackend = env.POKEPILOT_DEV_BACKEND || (target === 'spectator'
    ? 'http://localhost:18081'
    : 'http://localhost:18080')

  return {
    base: '/',
    plugins: [vue(), tailwindcss()],
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
