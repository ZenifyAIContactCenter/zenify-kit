---
summary: In các viewport width khai báo trong khối `uiVerify` của `.claude/worktree.json`, mỗi dòng một width.
---
## Khi nào dùng

Skill ui-verifier đọc danh sách này để biết cần chụp ở những độ rộng nào. Repo không khai báo `uiVerify` thì không in gì.

## Ví dụ

```bash
zenify ui-verify viewports --repo .
```
