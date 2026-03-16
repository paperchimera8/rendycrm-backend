import path from 'path'
import { fileURLToPath } from 'url'
import { defineConfig } from '@playwright/test'

const webRoot = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(webRoot, '../..')

const apiCommand = 'GOCACHE=/tmp/go-build-cache PORT=3000 APP_ENCRYPTION_SECRET=test-encryption-secret ENABLE_DEMO_SEED=1 go run ./cmd/api'

const webCommand = 'npm run dev -- --host 127.0.0.1'

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false,
  retries: 0,
  timeout: 60_000,
  use: {
    baseURL: 'http://127.0.0.1:5173',
    trace: 'on-first-retry',
    timezoneId: 'Europe/Moscow'
  },
  webServer: [
    {
      command: apiCommand,
      cwd: repoRoot,
      url: 'http://127.0.0.1:3000/health',
      reuseExistingServer: false,
      timeout: 120_000
    },
    {
      command: webCommand,
      cwd: webRoot,
      url: 'http://127.0.0.1:5173/login',
      reuseExistingServer: false,
      timeout: 120_000
    }
  ]
})
