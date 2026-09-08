import { test, expect } from '@playwright/test';
import { readFileSync } from 'fs';

// mask = selector các vùng động (badge notification, đồng hồ, call-status dot…) — Playwright
// tô đè khối màu đặc lên chúng trước khi so, nên nội dung thay đổi run-to-run không làm
// baseline flaky. App-shell dùng chung thường có 1-2 vùng như vậy ở header.
type Route = { name: string; path: string; waitFor?: string; mask?: string[] };

// Đọc danh mục route commit trong target repo (mount vào /harness/.znf/visual).
const routes: Route[] = JSON.parse(
  readFileSync('.znf/visual/routes.json', 'utf8'),
);

for (const r of routes) {
  test(r.name, async ({ page }) => {
    await page.goto(r.path);
    await page.evaluate(() => document.fonts.ready);
    if (r.waitFor) await page.waitForSelector(r.waitFor);
    await expect(page).toHaveScreenshot(`${r.name}.png`, {
      fullPage: true,
      animations: 'disabled',
      caret: 'hide',
      scale: 'css',
      mask: (r.mask ?? []).map((sel) => page.locator(sel)),
    });
  });
}
