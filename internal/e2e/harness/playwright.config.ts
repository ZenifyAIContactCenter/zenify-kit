// internal/e2e/harness/playwright.config.ts
import { defineConfig, devices } from '@playwright/test';

const STORAGE = process.env.STORAGE_STATE || '/tmp/znf-e2e-storage.json';

export default defineConfig({
  fullyParallel: false,
  reporter: [['list']],
  use: {
    baseURL: process.env.BASE_URL,
    trace: 'retain-on-failure',
  },
  outputDir: '.znf/e2e/__output__',
  projects: [
    { name: 'setup', testDir: '.', testMatch: /auth\.setup\.ts/ },
    {
      name: 'chromium',
      testDir: '.znf/e2e',
      testMatch: /.*\.spec\.ts/,
      use: { ...devices['Desktop Chrome'], storageState: STORAGE },
      dependencies: ['setup'],
    },
  ],
});
