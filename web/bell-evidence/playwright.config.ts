import { defineConfig } from '@playwright/test'
import { fileURLToPath } from 'node:url'

const here = fileURLToPath(new URL('.', import.meta.url))

export default defineConfig({
  testDir: here,
  testMatch: '**/*.playwright.ts',
  outputDir: `${here}../output/bell-evidence-tmp`,
  use: {
    baseURL: 'http://127.0.0.1:5199',
    viewport: { width: 1440, height: 900 },
    channel: 'chrome',
    headless: true,
    launchOptions: { slowMo: 200 },
    video: { mode: 'on', size: { width: 1440, height: 900 } },
  },
  retries: 0,
  workers: 1,
})
