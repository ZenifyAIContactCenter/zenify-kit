---
summary: Ghi và báo cáo các lượt cook/fix/hotfix cho pane dev track.
---
## Khi nào dùng

Các skill cook, fix, hotfix, ship và ci-watch gọi nhóm lệnh này để ghi dòng vào `<workspace>/.zenify/track/log.md`. Chỉ ghi khi bật `zenify feature track on`; khi tắt, mọi lệnh ghi in `track: off` và không ghi gì.

## Ví dụ

```bash
zenify track stats --json
```
