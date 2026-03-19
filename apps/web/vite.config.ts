import path from 'path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const devProxyTarget = process.env.VITE_DEV_PROXY_TARGET || 'http://127.0.0.1:3000'
const appBasePath = normalizeBasePath(process.env.VITE_APP_BASE_PATH)

function normalizeBasePath(value?: string) {
  const raw = (value || '').trim()
  if (!raw || raw === '/') {
    return '/'
  }
  const segments = raw.split('/').filter(Boolean)
  return segments.length ? `/${segments.join('/')}/` : '/'
}

export default defineConfig({
  base: appBasePath,
  plugins: [react()],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') }
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/test/setup.ts',
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
    exclude: ['tests/e2e/**']
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: devProxyTarget,
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, '')
      }
    }
  }
})
