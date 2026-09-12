import { defineConfig } from 'vite'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig(({ mode }) => {
  const target = mode === 'spectator' ? 'spectator' : 'operator'

  return {
    base: '/',
    plugins: [vue(), tailwindcss()],
    build: {
      outDir: `../cmd/pokeui/ui/vue/${target}`,
      emptyOutDir: true,
      rollupOptions: {
        input: `${target}.html`
      }
    }
  }
})
