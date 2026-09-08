import { test as setup } from '@playwright/test';

const STORAGE = process.env.STORAGE_STATE || '/tmp/znf-visual-storage.json';

// Login một lần → storageState (regenerate mỗi run, không commit token). Creds
// đọc từ env E2E_* truyền vào container. Luồng login cụ thể theo target repo —
// v1 nhắm contact-center-web (form email/password).
setup('authenticate', async ({ page }) => {
  const domain = process.env.E2E_DOMAIN!;
  await page.goto(`${domain}/login`);
  await page.fill('input[type="email"]', process.env.E2E_EMAIL!);
  await page.fill('input[type="password"]', process.env.E2E_PASSWORD!);
  await page.click('button[type="submit"]');
  await page.waitForLoadState('networkidle');
  await page.context().storageState({ path: STORAGE });
});
