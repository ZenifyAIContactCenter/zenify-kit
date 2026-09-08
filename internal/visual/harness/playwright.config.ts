import { defineConfig, devices } from '@playwright/test';

// Determinism: deviceScaleFactor cố định 1, chromium-only (v1). storageState do
// project 'setup' sinh ra rồi các project khác phụ thuộc.
const STORAGE = process.env.STORAGE_STATE || '/tmp/znf-visual-storage.json';

export default defineConfig({
  testDir: '.',
  fullyParallel: false,
  reporter: [['list']],
  use: {
    baseURL: process.env.BASE_URL,
    deviceScaleFactor: 1,
  },
  snapshotPathTemplate: '.znf/visual/__snapshots__/{arg}-{projectName}-{platform}{ext}',
  outputDir: '.znf/visual/__diff__',
  projects: [
    { name: 'setup', testMatch: /auth\.setup\.ts/ },
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'], storageState: STORAGE },
      dependencies: ['setup'],
      testMatch: /visual\.spec\.ts/,
    },
  ],
});
