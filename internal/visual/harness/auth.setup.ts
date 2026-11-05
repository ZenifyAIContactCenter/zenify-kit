import { test as setup, expect } from '@playwright/test';

const STORAGE = process.env.STORAGE_STATE || '/tmp/znf-visual-storage.json';

// Login một lần → storageState (regenerate mỗi run, không commit token). Creds đọc từ
// env E2E_* truyền vào container. Form login zenify chung cho mọi app contact-center:
// 3 field theo name (domain/email/password) — email là type=text nên KHÔNG dùng
// input[type="email"]. Nút submit là custom <Button> KHÔNG có type="submit" (render
// <button> thường) và nhãn theo i18n (Login/Đăng nhập), nên submit form bằng Enter trên
// field password: bám native <form onSubmit>, không phụ thuộc type nút lẫn ngôn ngữ.
// BASE_URL = origin app (dev-server host.docker.internal:<port>) là nơi login VÀ chụp,
// nên storageState (per-origin) áp đúng; E2E_DOMAIN = tenant slug điền field Domain
// (KHÔNG phải URL).
setup('authenticate', async ({ page }) => {
  await page.goto(`${process.env.BASE_URL}/login`);
  await page.fill('input[name="domain"]', process.env.E2E_DOMAIN!);
  await page.fill('input[name="email"]', process.env.E2E_EMAIL!);
  await page.fill('input[name="password"]', process.env.E2E_PASSWORD!);
  await page.press('input[name="password"]', 'Enter');
  await page.waitForLoadState('networkidle');
  // A failed login's error-context.md is an aria snapshot of every input value, taken
  // when expect fails: empty the password now. fill() fires an input event, so React's
  // state is emptied too and a re-render cannot put the value back. The form was
  // already submitted; after a successful login the field is gone (count() is 0).
  const password = page.locator('input[name="password"]');
  if (await password.count().catch(() => 0)) {
    await password.fill('', { timeout: 2000 }).catch(() => {});
  }
  await expect(page).not.toHaveURL(/\/login(\?|$)/);
  await page.context().storageState({ path: STORAGE });
});
