import { test as setup } from '@playwright/test';

const STORAGE = process.env.STORAGE_STATE || '/tmp/znf-visual-storage.json';

// Login một lần → storageState (regenerate mỗi run, không commit token). Creds
// đọc từ env E2E_* truyền vào container. Luồng login cụ thể theo target repo —
// v1 nhắm contact-center-web (form email/password).
setup('authenticate', async ({ page }) => {
  // Login trên CÙNG origin sẽ chụp (BASE_URL = host.docker.internal:<port>) — storageState
  // per-origin, khác origin thì session không áp. E2E_DOMAIN chỉ là override tường minh.
  const base = process.env.BASE_URL || process.env.E2E_DOMAIN!;
  await page.goto(`${base}/login`);
  await page.fill('input[type="email"]', process.env.E2E_EMAIL!);
  await page.fill('input[type="password"]', process.env.E2E_PASSWORD!);
  await page.click('button[type="submit"]');
  await page.waitForLoadState('networkidle');
  await page.context().storageState({ path: STORAGE });
});
