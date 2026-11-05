---
summary: Gỡ phần ZenifyKit đã cài vào máy và repo, giữ nguyên code, workspace và knowledge store.
---
## Khi nào dùng

Khi bạn rời workspace, đổi máy, hoặc cần cài lại từ đầu. Đây là bước đảo của `zenify up`.

## Kết quả

Lệnh gỡ phần kit cài ở workspace: plugin `znf@zenify-kit` và marketplace `zenify-kit` (scope local), ba MCP server `zenify-grafana`, `zenify-posthog`, `zenify-plane` (`-s local`), hook `znf`, hook `git-guard`, biến `env` `CLAUDE_CODE_SUBAGENT_MODEL` và `ZNF_REVIEWER` cùng các luật `permissions` và `sandbox.excludedCommands` của kit trong `<workspace>/.claude/settings.local.json`, các file Codex (`<workspace>/.codex/hooks.json`, `<workspace>/.codex/agents`, link `<workspace>/.agents/skills/znf`) và mục trust của workspace trong `~/.codex/config.toml`. Lệnh cũng gỡ mọi bản cài cấp user còn sót của kit bản cũ (`~/.claude/skills/znf`, hook, MCP `-s user`, file Codex ở `~/`). Ngoài ra lệnh gỡ các dòng exclude `.worktrees/` và `.wt/` trong từng repo, file `.claude/settings.local.json` do kit sinh ra trong từng repo, và luật deny `Read(//<workspace>/.zenify/.env)` kit thêm vào `~/.claude/settings.json` cho workspace đó (luật bạn tự thêm và luật của workspace khác giữ nguyên). Mặc định lệnh chỉ in preview có tiêu đề `PREVIEW`. Với `--apply` lệnh thực thi; chạy lại khi không còn gì để gỡ vẫn an toàn.

Lệnh không đụng vào repo đã clone, thư mục `.zenify/`, knowledge store, và chính file `<workspace>/.claude/settings.local.json` của workspace (chỉ các key kit sở hữu bị gỡ; key của bạn như `autoMemoryDirectory` còn nguyên). Skill và hook cá nhân trong `~/.claude` giữ nguyên.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--apply` | Thực thi thay đổi. Không có cờ này lệnh chỉ in preview. |
| `--manifest` | Đường dẫn manifest workspace, khi không dùng bản mặc định. |
| `--overlay` | File overlay cho máy này. |
| `--workspace` | Thư mục workspace, mặc định là thư mục hiện tại. |

## Ví dụ

```bash
zenify down
zenify down --apply
```
