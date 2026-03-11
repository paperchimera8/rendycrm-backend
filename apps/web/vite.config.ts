import path from 'path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
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
      '/auth': 'http://127.0.0.1:8080',
      '/dashboard': 'http://127.0.0.1:8080',
      '/conversations': 'http://127.0.0.1:8080',
      '/customers': 'http://127.0.0.1:8080',
      '/availability': 'http://127.0.0.1:8080',
      '^/slots/.+': 'http://127.0.0.1:8080',
      '/slots/editor': 'http://127.0.0.1:8080',
      '/slots/settings': 'http://127.0.0.1:8080',
      '/slots/colors': 'http://127.0.0.1:8080',
      '/slots/templates': 'http://127.0.0.1:8080',
      '/slots/day-slots': 'http://127.0.0.1:8080',
      '/slots/available': 'http://127.0.0.1:8080',
      '/slot-holds': 'http://127.0.0.1:8080',
      '/bookings': 'http://127.0.0.1:8080',
      '/reviews': 'http://127.0.0.1:8080',
      '/analytics': 'http://127.0.0.1:8080',
      '/settings': 'http://127.0.0.1:8080',
      '/events': 'http://127.0.0.1:8080',
      '/webhooks': 'http://127.0.0.1:8080',
      '/health': 'http://127.0.0.1:8080'
    }
  }
})
