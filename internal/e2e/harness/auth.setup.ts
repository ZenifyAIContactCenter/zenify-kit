// internal/e2e/harness/auth.setup.ts
import { test as setup } from '@playwright/test';

const STORAGE = process.env.STORAGE_STATE || '/tmp/znf-e2e-storage.json';

// Login một lần → storageState (regenerate mỗi run, không commit token). Form login zenify:
// 3 field theo name (domain/email/password), email type=text (KHÔNG dùng input[type=email]),
// nút submit custom KHÔNG có type=submit và nhãn theo i18n → submit bằng Enter trên password.
setup('authenticate', async ({ page }) => {
  await page.goto(`${process.env.BASE_URL}/login`);
  await page.fill('input[name="domain"]', process.env.E2E_DOMAIN!);
  await page.fill('input[name="email"]', process.env.E2E_EMAIL!);
  await page.fill('input[name="password"]', process.env.E2E_PASSWORD!);
  await page.press('input[name="password"]', 'Enter');
  await page.waitForLoadState('networkidle');
  await page.context().storageState({ path: STORAGE });
});
