---
title: Ba lớp của kit
---

# Ba lớp: binary, plugin, knowledge store

## Vấn đề nó giải quyết

`zenify-kit` không phải một thứ cài một lần rồi xong. Nó là ba thứ tách biệt, mỗi thứ sống ở một chỗ khác nhau trên máy bạn, cập nhật bằng lệnh khác nhau, do một bên khác nhau sửa. Không phân biệt ba lớp, bạn dễ sửa nhầm chỗ — ví dụ sửa tay một skill trong `~/.claude/skills/znf` tưởng đang tuỳ biến workflow riêng, rồi ngạc nhiên khi bản sửa biến mất sau lần đồng bộ kế tiếp. Trang này nói ba lớp là gì, ai chịu trách nhiệm từng lớp, vì sao ranh giới đó phải rõ.

## Mô hình tư duy

| Lớp | Vị trí trên máy | Cập nhật bằng | Ai sửa |
|---|---|---|---|
| Binary `zenify` | trong `PATH` | `zenify update` | release của kit (tag + goreleaser) |
| Plugin skill `znf:*` | `~/.claude/skills/znf` | `zenify skills sync` (tự chạy trong `zenify up`) | PR vào repo kit — nội dung skill nhúng trong binary qua `go:embed` |
| Knowledge store | `~/.zenify/knowledge` | hook `Stop`/`SessionStart` gọi `zenify docs sync` | agent, khi viết spec/plan/handoff/release trong phiên làm việc |

Ba lớp không đồng bộ với nhau: binary có thể ở `v0.17.7` trong khi knowledge store vừa nhận một commit agent viết một phút trước. Trộn chúng — coi plugin skill như thứ bạn tự viết, hoặc coi knowledge store như một phần binary — là nhầm lẫn phổ biến nhất khi mới dùng kit.

## Ghép với …

- [Namespace `znf:`](/concepts/znf-namespace) — vì sao lớp plugin không đụng vào skill cá nhân của bạn.
- [Knowledge store và view `docs/`](/concepts/knowledge-store) — chi tiết lớp thứ ba, vì sao nó không nằm trong repo kit.
- [Cài đặt](/getting-started/install) — nơi lớp binary được cài lần đầu.

## Edge case

`zenify skills sync` ghi cây `znf/` theo manifest theo dõi từng file, nên lần chạy lại **không đối xứng**: file chưa đụng tới được cập nhật lên bản mới nhất trong binary; file **đã sửa tay** được giữ nguyên, không ghi đè. Nghe an toàn nhưng có bẫy ngược: sửa tay xong, bạn không còn nhận bản vá tiếp theo của file đó — mọi cải tiến từ `zenify update` sau này lặng lẽ bỏ qua nó, tới khi bạn xoá bản sửa tay hoặc đưa thay đổi ngược vào repo kit qua PR. Muốn sửa hành vi một skill kit lâu dài, sửa ở repo kit — không sửa bản đã materialize trên máy.

## Nguồn

- `internal/plugin/assets/znf/skills/discipline/SKILL.md` (bố cục plugin)
- `docs/handoff/zenify-kit/m2-plugin-skills.md` (`zenify skills sync` additive và refresh-safe: cập nhật file chưa sửa tay, giữ nguyên file đã sửa tay)
- `docs/handoff/zenify-kit/docs-sync-layer.md` (lớp knowledge store, agent là writer duy nhất)
- Ground trên binary build từ commit bf91c62 của nhánh này (2026-09-14), chưa phát hành.
