import { defineConfig } from 'vitest/config'

export default defineConfig({
  server: {
    proxy: {
      '/api': { target: 'http://127.0.0.1:7842', changeOrigin: true },
      '/stream': { target: 'http://127.0.0.1:7842', changeOrigin: true },
    },
  },
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
  },
  test: {
    environment: 'happy-dom',
  },
})
