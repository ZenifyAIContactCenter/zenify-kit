// internal/e2e/harness/auth.setup.ts
import { test as base, expect } from '@playwright/test';

const STORAGE = process.env.STORAGE_STATE || '/tmp/znf-e2e-storage.json';

// Login một lần → storageState (regenerate mỗi run, không commit token). Form login zenify:
// 3 field theo name (domain/email/password), email type=text (KHÔNG dùng input[type=email]),
// nút submit custom KHÔNG có type=submit và nhãn theo i18n → submit bằng Enter trên password.
// A failed or timed-out login leaves error-context.md, an aria snapshot of every input
// value taken when the browser context closes. This page teardown runs before that
// close, also after a test timeout, and empties the password first: the native value
// setter plus an input event (React's state follows), in page script so a disabled or
// readonly field is emptied too and nothing waits. Scoped to this file: journeys keep
// their full snapshot.
const setup = base.extend({
  page: async ({ page }, use) => {
    await use(page);
    await page
      .locator('input[name="password"]')
      .evaluateAll((els) => {
        const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
        for (const el of els as HTMLInputElement[]) {
          set.call(el, '');
          el.dispatchEvent(new Event('input', { bubbles: true }));
        }
      })
      .catch(() => {});
  },
});

setup('authenticate', async ({ page }) => {
  await page.goto(`${process.env.BASE_URL}/login`);
  await page.fill('input[name="domain"]', process.env.E2E_DOMAIN!);
  await page.fill('input[name="email"]', process.env.E2E_EMAIL!);
  // A failed fill() prints its value in the error: rethrow without it.
  await page.fill('input[name="password"]', process.env.E2E_PASSWORD!).catch(() => {
    throw new Error('could not fill input[name="password"] (value withheld)');
  });
  await page.press('input[name="password"]', 'Enter');
  await page.waitForLoadState('networkidle');
  await expect(page).not.toHaveURL(/\/login(\?|$)/);
  await page.context().storageState({ path: STORAGE });
});
