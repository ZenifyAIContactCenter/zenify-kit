---
title: zenify up
---

[Lệnh CLI](/reference/cli/) / `zenify up`

# `zenify up`

Onboard máy vào workspace: clone repo, ghi cấu hình, gắn hook và skill, chuẩn bị knowledge store.

## Cú pháp

```text
zenify up [flags]
```

## Khi nào dùng

Lần đầu cài ZenifyKit trên một máy, hoặc khi team thêm repo mới và bạn cần kéo về đúng bộ. Chạy lại bất cứ lúc nào đều an toàn: lệnh chỉ sửa những gì lệch so với cấu hình chung.

## Kết quả

Chạy trong terminal, lệnh mở wizard tương tác: chọn thư mục workspace, chọn repo trong danh sách bạn có quyền truy cập, rồi nhập các secret cần cho verify UI và đọc DB. Chạy ngoài terminal, lệnh chỉ in kế hoạch dạng bảng `REPO STATE REASON` và dừng.

Khi apply, lệnh clone repo còn thiếu, ghi các file cấu hình do kit sở hữu và cài kit vào gốc workspace, không vào cấp user: plugin `znf@zenify-kit` từ marketplace thư mục `zenify-kit` tại `~/.zenify/plugin-marketplace` (hoặc `$ZENIFY_HOME/plugin-marketplace`) cài `--scope local`; hook `znf`, hook `git-guard` (chặn agent commit/push vào branch deploy), `env.CLAUDE_CODE_SUBAGENT_MODEL=sonnet` và `env.ZNF_REVIEWER` (chỉ khi chưa có; giá trị bạn đã đặt ở `~/.claude/settings.json` được chép sang thay cho mặc định) và luật `permissions` của kit ghi vào `<workspace>/.claude/settings.local.json`; ba MCP server `zenify-grafana`, `zenify-posthog`, `zenify-plane` đăng ký `-s local`. Chạy trong terminal mà chưa có `PLANE_API_KEY` thì lệnh mở trang tạo Plane token (`<plane>/settings/profile/api-tokens/`), hỏi token bằng ô nhập ẩn, kiểm với Plane (in tên chủ token) rồi lưu vào `<workspace>/.zenify/.env` trước khi đăng ký `zenify-plane`; không có terminal thì chỉ in gợi ý kèm link. Khi có `codex` trên PATH, Codex nhận `<workspace>/.codex/hooks.json`, `<workspace>/.codex/agents`, link `<workspace>/.agents/skills/znf` và một mục trust trong `~/.codex/config.toml`; bạn cần duyệt hook mới qua `/hooks` trong Codex. Luật deny đọc `.zenify/.env` vẫn ở `~/.claude/settings.json`. Vì vậy kit chỉ hoạt động khi session mở ở gốc workspace; ngoài workspace và trong thư mục con của nó, kit không bật. Sau khi mọi bước cài local thành công, lệnh gỡ bản cài cấp user của kit bản cũ (`~/.claude/skills/znf`, hook và luật kit trong `~/.claude/settings.json`, MCP `-s user` của kit đã có bản `-s local` thay, file Codex ở `~/`; MCP bị bỏ qua đăng ký giữ bản `-s user`, và MCP bạn tự cấu hình dưới cùng tên cũng giữ; riêng khi chưa đăng ký được `zenify-plane` local, ví dụ chưa có `PLANE_API_KEY` hoặc `uv`, mà còn `zenify-plane` cấp user của kit, lệnh coi đó là bước local lỗi để `plane-guard` cấp user không bị gỡ). `~/.claude/skills/znf` có file bạn đã sửa hoặc thêm thì được chuyển sang `~/.zenify/backup/skills-znf-<thời điểm>` thay vì xoá, và lệnh in đường dẫn đó. Hook, agent và link skill Codex ở `~/` chỉ bị gỡ khi workspace đã có bản thay (có `codex` trên PATH, và `<workspace>/.agents/skills/znf` là link của kit). Rule kit bạn đã tự xoá ở cấp user không bị thêm lại vào workspace; skill và hook cá nhân trong `~/.claude` giữ nguyên. Bước gỡ là tất cả hoặc không gì: nếu một bước local lỗi, kể cả máy không có `claude` trên PATH, lệnh không gỡ gì, giữ bản cài cấp user, gỡ lại hook `znf`, `git-guard` và hook Codex vừa ghi vào workspace (để hook không chạy hai lần), in lỗi và thoát khác 0; lần `up` sau thử lại. Nếu chính bước gỡ bản cấp user dừng giữa chừng, hook nào còn bản cấp user thì bản trong workspace của nó được gỡ lại, và các SessionStart sau cũng không ghi lại hook đó cho tới khi bản cấp user hết. Khi bước local lỗi, lệnh cũng chưa đăng ký `zenify-plane` (hook `plane-guard` của nó nằm trong workspace); lần `up` sau đăng ký. Preview (`--dry-run`) liệt kê những mục sẽ gỡ. Lệnh cũng chuẩn bị Playwright nếu workspace có repo frontend, clone knowledge store và ghi con trỏ workspace vào `~/.zenify/workspace`.

Lệnh cần `gh` đã đăng nhập với scope `read:org` và `repo`. Chưa đăng nhập, lệnh dừng và nhắc chạy `gh auth login`.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--apply` | Thực thi kế hoạch. Không kèm cờ này và không ở terminal, lệnh chỉ in kế hoạch. |
| `--dry-run` | Chỉ in kế hoạch, không đổi gì. Đây là mặc định ngoài terminal. Không dùng cùng `--apply`. |
| `--json` | In kế hoạch dạng JSON thay cho bảng. |
| `--manifest` | Đường dẫn manifest workspace, khi không dùng bản mặc định trong `.zenify/`. |
| `--non-interactive` | Không mở wizard kể cả khi đang ở terminal. Dùng trong script. |
| `--overlay` | File overlay cho máy này, mặc định `.zenify-overlay.yaml` trong workspace. |
| `--reviewer` | Đặt engine review `agent` hoặc `codex` vào `ZNF_REVIEWER` trong `<workspace>/.claude/settings.local.json`. Chỉ ghi khi có `--apply`. |
| `--workspace` | Thư mục workspace. Không có, wizard hỏi hoặc lệnh dùng thư mục hiện tại. |

## Ví dụ

```bash
zenify up
zenify up --non-interactive --dry-run --workspace ~/WorkingSpace/zenify
zenify up --non-interactive --apply --workspace ~/WorkingSpace/zenify
```

## Lưu ý

Toàn bộ đường đi từ cài binary đến session đầu tiên: [Bắt đầu nhanh](/getting-started/quickstart). Sau `zenify up`, chạy `zenify doctor` để xác nhận môi trường.

## Lệnh liên quan

- [zenify](./zenify): Binary dòng lệnh của ZenifyKit, chứa mọi lệnh workspace và plugin skill đi kèm.

<!-- Generated by zenify docs gen: prose from internal/docsgen/catalog, facts from the command tree. -->
