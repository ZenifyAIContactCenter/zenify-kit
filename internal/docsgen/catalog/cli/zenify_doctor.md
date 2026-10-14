---
summary: Kiểm tra sức khỏe môi trường ZenifyKit trên máy và báo từng mục đạt hay hỏng.
---
## Khi nào dùng

Ngay sau `zenify up`, và mỗi khi một skill báo thiếu tool, thiếu secret hoặc Playwright không chạy. Lệnh chỉ đọc, chạy bao nhiêu lần cũng được.

## Kết quả

Mỗi dòng là một mục kiểm tra kèm dấu đạt hoặc hỏng và chi tiết. Các mục gồm phiên bản binary, đăng nhập git và quyền truy cập GitHub, danh tính commit của git (`user.name`, `user.email`), credential trong `.zenify/.env` (`env-file`, `settings-env`), tool bên ngoài (`git`, `gh`, `mongosh`, `mysql`), Playwright, Docker, hook `git-guard`, plugin skill `znf`, hook Codex (`codex-hooks`), engine review (`reviewer`) và file `AGENTS.md` của từng repo (`agents-md`), skill, agent và rule cá nhân đè bản của kit hoặc workspace (`shadow`) dung lượng file hướng dẫn luôn nạp (`context-budget`), entry ngoài quy ước ở root và `docs/` (`workspace-root`) và MEMORY.md gần trần (`memory-index`). Mục hỏng in luôn chi tiết để bạn sửa. Mục `git-guard` hỏng thì `--fix` tự gắn lại hook.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--exit-on-fail` | Trả mã thoát khác 0 khi có mục hỏng. Dùng trong script và CI. |
| `--fix` | Tự sửa các mục an toàn rồi kiểm tra lại. Những mục cần bạn tự làm vẫn in hướng dẫn. |
| `--json` | In kết quả dạng JSON. |

## Ví dụ

```bash
zenify doctor
zenify doctor --fix
```

## Lưu ý

Credential được đọc từ `.zenify/.env` ở workspace, nên phải mở session từ trong workspace. Xem [Bắt đầu nhanh](/getting-started/quickstart) để biết từng mục nghĩa gì.
