import { fileURLToPath, URL } from 'node:url'
/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// ds-ui API for `npm run dev`; override with DSUI_API=http://host:port
const api = process.env.DSUI_API ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [vue()],
  define: {
    // vue-i18n bundler flags: Composition API only, runtime message compiler kept.
    __VUE_I18N_FULL_INSTALL__: false,
    __VUE_I18N_LEGACY_API__: false,
    __INTLIFY_PROD_DEVTOOLS__: false,
    __INTLIFY_DROP_MESSAGE_COMPILER__: false,
  },
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  build: {
    // Embedded into the Go binary by internal/webui.
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
    // Admin UI on a local network: one ~160 KB gzip bundle is fine.
    chunkSizeWarningLimit: 800,
  },
  // Unit tests only; the browser tests in e2e/ run with Playwright.
  test: {
    include: ['src/**/*.{test,spec}.ts'],
  },
  server: {
    proxy: {
      '/api': api,
      '/healthz': api,
      '/readyz': api,
    },
  },
})
