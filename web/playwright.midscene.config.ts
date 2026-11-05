import { config as loadEnvironment } from 'dotenv'
import { resolve } from 'node:path'
import { env } from 'node:process'
import { fileURLToPath } from 'node:url'
import { defineConfig } from '@playwright/test'

const webRoot = fileURLToPath(new URL('.', import.meta.url))
loadEnvironment({ path: resolve(webRoot, '.env.midscene.local') })

const targetBaseURL = env.MIDSCENE_BASE_URL ?? 'http://127.0.0.1:4177'
const usesLocalVite = env.MIDSCENE_BASE_URL === undefined

export default defineConfig({
  testDir: './midscene',
  testMatch: '**/*.spec.ts',
  timeout: 120_000,
  expect: { timeout: 8_000 },
  reporter: [
    ['list'],
    ['@midscene/web/playwright-reporter', { type: 'merged', outputFormat: 'single-html' }],
  ],
  outputDir: '../output/midscene/playwright',
  use: {
    baseURL: targetBaseURL,
    browserName: 'chromium',
    launchOptions: { channel: 'chrome' },
    viewport: { width: 1440, height: 900 },
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  ...(usesLocalVite
    ? {
        webServer: {
          command: 'node_modules/.bin/vite --host 127.0.0.1 --port 4177 --strictPort',
          url: `${targetBaseURL}/login`,
          reuseExistingServer: false,
          timeout: 30_000,
        },
      }
    : {}),
})
