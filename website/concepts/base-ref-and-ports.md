---
title: Base ref và port block
---

# Base ref, hotfix base, port block

## Vấn đề nó giải quyết

Mỗi repo trong workspace có một nhánh tích hợp riêng (`staging`, `develop`, `main`, …), và một cách khác để tính base cho hotfix (release mới nhất, không phải nhánh tích hợp). Nếu một skill hardcode tên nhánh, nó đúng cho repo này và sai lặng lẽ ở repo kế bên. `.claude/worktree.json` giải quyết việc đó bằng cách khai báo base như một **fact của repo**, đọc ra khi cần chứ không đoán. Cùng file đó còn cấp mỗi repo một khối port riêng, để nhiều worktree ở nhiều repo chạy song song không bao giờ đụng cổng nhau.

## Mô hình tư duy

Ví dụ thật, `.claude/worktree.json` của chính repo `zenify-kit`:

```json
{
  "abbrev": "kit",
  "baseRef": "origin/main",
  "worktreeDir": ".worktrees/",
  "copy": [".claude/settings.local.json"],
  "deps": "none",
  "portEnv": "PORT",
  "portRange": [3750, 3799]
}
```

| Trường | Ý nghĩa |
|---|---|
| `baseRef` | nhánh gốc mặc định cho `wt new --type feat/fix` |
| `portRange` | khối 50 port riêng của repo này (3750–3799) |
| `portEnv` | tên biến môi trường worktree đọc port từ đó |
| `deps` | `none` ở đây vì kit build bằng Go, không cần seed `node_modules` |

Hotfix không dùng `baseRef` — nó resolve base **sau khi fetch**, từ nhánh release mới nhất (`release-latest`) hoặc một chiến lược khác khai báo trong `hotfix.baseStrategy`. `zenify hotfix baseref <repoPath>` in ra base đã resolve theo đúng chiến lược của repo đó, để một skill không phải tự đoán pattern `release[0-9]+`. `wt url <slug>` in `http://localhost:<port>` đã cấp, nên bạn không cần nhớ port của từng worktree.

## Ghép với …

- [Worktree theo slug](/concepts/worktree-per-slug) — nơi `--base` được dùng.
- Tham chiếu lệnh: [`zenify wt config`](/reference/cli/zenify_wt_config).

## Edge case

- **Fetch trước, resolve sau — thứ tự không đảo được.** Nếu resolve release mới nhất trước khi fetch, bạn có thể bỏ lỡ một release vừa cắt sáng nay và branch hotfix nhầm từ bản cũ — nhìn vẫn đúng cho tới khi merge. `wt new` bản hiện tại tự fetch trước khi resolve base, nhưng một bản cũ có thể chưa làm vậy, nên vẫn nên fetch tay trước khi gọi.
- **Hai repo trùng port block** làm hai worktree ở hai repo khác nhau tranh nhau một cổng khi chạy song song. Khi thêm repo mới, đọc `portRange` của các repo lân cận trước khi chọn khối tiếp theo — đừng chọn tự do.

## Nguồn

- `.claude/worktree.json` (repo `zenify-kit`, đọc trực tiếp)
- `./zenify wt config --help`
- `docs/handoff/zenify-kit/m2-plugin-skills.md` (`hotfix.baseStrategy`, `zenify hotfix baseref`, port range 50-port/repo)
- `internal/plugin/assets/znf/skills/discipline/SKILL.md` §8 (base là fact khai báo, hotfix resolve sau fetch)
- Ground trên binary build từ commit bf91c62 của nhánh này (2026-09-14), chưa phát hành.
