// internal/e2e/harness/fixtures.ts
// Fixtures generic project-agnostic. Mọi thứ zenify-specific nằm ở .znf/e2e/e2e.config.json (repo-owned).
import { test as base, expect, request, type APIRequestContext, type Page } from '@playwright/test';
import { readFileSync } from 'fs';

// contactId optional: journey nên resolve entity phụ (vd requester contact) TẠI RUNTIME
// qua API — id pin cứng trong config có thể bị soft-delete và làm test rot âm thầm. Giữ
// field cho repo nào muốn pin, nhưng không bắt buộc (config thật có thể chỉ có apiBaseUrl).
type E2EConfig = { apiBaseUrl: string; contactId?: string };

function loadConfig(): E2EConfig {
  return JSON.parse(readFileSync('.znf/e2e/e2e.config.json', 'utf8'));
}

// Trích JWT từ file storageState do auth.setup ghi (origins[].localStorage 'user'.token).
// Đọc từ file thay vì page.evaluate để không phụ thuộc thứ tự navigate.
function tokenFromStorage(): string {
  const path = process.env.STORAGE_STATE || '/tmp/znf-e2e-storage.json';
  try {
    const st = JSON.parse(readFileSync(path, 'utf8'));
    for (const o of st.origins || []) {
      const u = (o.localStorage || []).find((e: { name: string }) => e.name === 'user');
      if (u) return JSON.parse(u.value).token || '';
    }
  } catch {
    /* fall through */
  }
  return '';
}

export type CleanupTracker = { add(undo: () => Promise<void>): void };

type Fixtures = {
  cfg: E2EConfig;
  apiClient: APIRequestContext;
  cleanupTracker: CleanupTracker;
  authedPage: Page;
};

export const test = base.extend<Fixtures>({
  cfg: async ({}, use) => {
    await use(loadConfig());
  },
  // apiClient: HTTP đã auth. Token thô trong Authorization (khớp be loggedIn.js — KHÔNG Bearer).
  apiClient: async ({ cfg }, use) => {
    const token = tokenFromStorage();
    const ctx = await request.newContext({
      baseURL: cfg.apiBaseUrl,
      extraHTTPHeaders: token ? { Authorization: token } : {},
    });
    await use(ctx);
    await ctx.dispose();
  },
  // cleanupTracker: undo theo thứ tự ngược ở cuối test; undo lỗi chỉ cảnh báo, không sập test khác.
  cleanupTracker: async ({}, use) => {
    const undos: Array<() => Promise<void>> = [];
    await use({ add: (u) => undos.push(u) });
    for (const u of undos.reverse()) {
      try {
        await u();
      } catch (e) {
        console.warn('[e2e cleanup] undo failed:', e);
      }
    }
  },
  // authedPage: passthrough của `page` (đã login qua project storageState, xem Step 3) —
  // khớp tên fixture FR-3.1 nêu, cùng đối tượng, không hành vi mới.
  authedPage: async ({ page }, use) => {
    await use(page);
  },
});

export { expect };
