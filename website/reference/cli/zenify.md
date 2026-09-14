---
title: zenify
---

## zenify

zenify — bộ công cụ workspace dùng chung của team

### Options

```
  -h, --help   help for zenify
```

### SEE ALSO

* [zenify analyze](./zenify_analyze)	 - Phân tích cơ học cặp spec+plan (coverage, marker, cấu trúc Brief) — advisory, fail-open
* [zenify config](./zenify_config)	 - phân phối config workspace-level từ docs/.config (dry-run mặc định)
* [zenify db-perf](./zenify_db-perf)	 - Quét tĩnh một diff tìm anti-pattern query (hai tầng) — advisory, fail-open
* [zenify db-read](./zenify_db-read)	 - Đọc chỉ-đọc các database dùng chung của zenify (không bao giờ ghi, không in secret)
* [zenify docs](./zenify_docs)	 - quản lý docs layer (agent-managed, dev read-only)
* [zenify doctor](./zenify_doctor)	 - Kiểm tra sức khoẻ môi trường, chỉ-đọc (không sửa gì, không in secret; --fix áp tập sửa an toàn)
* [zenify down](./zenify_down)	 - Offboard: gỡ znf global hooks, .worktrees/ + .wt/ excludes, và owned settings skeletons (preview mặc định; --apply để thực thi)
* [zenify e2e](./zenify_e2e)	 - E2E functional (Playwright journey thật trong Docker) + lint chống test hợt
* [zenify gate](./zenify_gate)	 - trợ giúp gate (contract sweep)
* [zenify guard](./zenify_guard)	 - Quản lý git-guard hook
* [zenify hotfix](./zenify_hotfix)	 - trợ giúp hotfix
* [zenify migrate](./zenify_migrate)	 - gom repo vào một thư mục con (dry-run mặc định; move-and-repair: gom được cả repo dirty và repo còn worktree)
* [zenify observe](./zenify_observe)	 - Observability: đếm/nhắc fan-out subagent
* [zenify release-note](./zenify_release-note)	 - ghi note-commit release (trailer risk-metadata) — dùng bởi /ship
* [zenify release-report](./zenify_release-report)	 - sinh report rủi ro cho một release (chỉ-đọc, ghi docs/releases/R\<N\>.md)
* [zenify review-log](./zenify_review-log)	 - Xem learning-capture log của znf:review (summary local; --json cho M6)
* [zenify rules](./zenify_rules)	 - quản lý và kiểm rule team (F1/F2/F3)
* [zenify secret-scan](./zenify_secret-scan)	 - Quét secret trong cây thư mục (dùng cho CI + kiểm tra tay)
* [zenify skills](./zenify_skills)	 - quản lý plugin skill znf
* [zenify spec](./zenify_spec)	 - soi vòng đời spec (planned/built) và registry contract từ store spec
* [zenify standards](./zenify_standards)	 - Kiểm test-traceability — mỗi FR có một test thật trên đĩa (advisory, fail-open)
* [zenify up](./zenify_up)	 - Onboard workspace: wizard tương tác trong terminal, còn không thì in kế hoạch dry-run (--apply để chạy headless)
* [zenify update](./zenify_update)	 - Nâng zenify lên bản release mới nhất (brew / scoop / install script)
* [zenify version](./zenify_version)	 - In version của zenify
* [zenify visual](./zenify_visual)	 - Visual-regression golden-diff (Playwright chạy trong Docker, đã ghim phiên bản)
* [zenify wt](./zenify_wt)	 - Quản lý git worktree + môi trường dev: tạo, liệt kê, gỡ, dọn worktree theo slug

