import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  resolve: {
    alias: {
      '@': path.join(path.dirname(fileURLToPath(import.meta.url)), 'src'),
    },
  },
  test: { environment: 'jsdom' },
})
