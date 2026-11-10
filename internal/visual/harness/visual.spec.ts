import { test, expect } from '@playwright/test';
import { readFileSync, mkdirSync, writeFileSync } from 'fs';

// mask = selector các vùng động (badge notification, đồng hồ, call-status dot…) — Playwright
// tô đè khối màu đặc lên chúng trước khi so, nên nội dung thay đổi run-to-run không làm
// baseline flaky. App-shell dùng chung thường có 1-2 vùng như vậy ở header.
type Route = { name: string; path: string; waitFor?: string; mask?: string[]; aria?: string };

// Đọc danh mục route commit trong target repo (mount vào /harness/.znf/visual).
const routes: Route[] = JSON.parse(
  readFileSync('.znf/visual/routes.json', 'utf8'),
);

for (const r of routes) {
  test(r.name, async ({ page }) => {
    await page.goto(r.path);
    await page.evaluate(() => document.fonts.ready);
    if (r.waitFor) await page.waitForSelector(r.waitFor);
    if (r.aria) {
      await expect.soft(page.locator(r.aria)).toMatchAriaSnapshot({ name: `${r.name}.aria.yml` });
    }
    await expect.soft(page).toHaveScreenshot(`${r.name}.png`, {
      fullPage: true,
      animations: 'disabled',
      caret: 'hide',
      scale: 'css',
      mask: (r.mask ?? []).map((sel) => page.locator(sel)),
    });
    // Advisory only: nothing here may fail the route.
    try {
      const { default: AxeBuilder } = await import('@axe-core/playwright');
      const { violations } = await new AxeBuilder({ page }).analyze();
      const out = violations.map((v) => ({
        id: v.id, impact: v.impact, help: v.help, nodes: v.nodes.length,
        targets: v.nodes.slice(0, 3).map((n) => n.target.join(' ')),
      }));
      mkdirSync('.znf/visual/__a11y__', { recursive: true });
      writeFileSync(`.znf/visual/__a11y__/${r.name}.json`, JSON.stringify({ route: r.name, violations: out }, null, 2) + '\n');
      const count = (impact: string) => out.filter((v) => v.impact === impact).length;
      console.log(`a11y ${r.name}: ${out.length} violation(s) (${count('critical')} critical, ${count('serious')} serious)`);
    } catch (e) {
      console.log(`a11y ${r.name}: skipped (${String((e as Error).message ?? e).split('\n')[0].slice(0, 120)})`);
    }
  });
}
