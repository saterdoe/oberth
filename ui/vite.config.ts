import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: { host: '127.0.0.1', port: 5173, proxy: { '/api': { target: 'http://127.0.0.1:9090', changeOrigin: true }, '/ws': { target: 'ws://127.0.0.1:9090', ws: true, changeOrigin: true } } },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/test-setup.ts',
    coverage: {
      provider: 'v8',
      reportsDirectory: '../artifacts/coverage/ui',
      reporter: ['text', 'json-summary', 'lcov'],
      thresholds: { statements: 65, branches: 45, functions: 55, lines: 85 },
    },
  },
})
