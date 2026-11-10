---
summary: Ghi lại artifact UI-verify (screenshot + số đo layout) cho fingerprint hiện tại.
---
## Khi nào dùng

Agent `ui-verifier` gọi lệnh này ngay trước khi xoá screenshot của nó, nếu `zenify` có trên PATH (guard giữ agent project-agnostic). Không tự chạy lệnh này trong `/znf:ship` — nó là mắt xích ghi bằng chứng, `check` mới là mắt xích gate.

## Kết quả

Lệnh tính fingerprint `fp` trên working tree của `--repo`, copy ảnh vào `<repo>/.znf/ui-verify/<fp>-<screen>-<viewport>.png` (ký tự ngoài `A-Z a-z 0-9 . _ -` trong tên screen, như `/` của một route, được đổi thành `-` và thêm 8 ký tự hash để hai tên khác nhau không ghi đè ảnh của nhau), rồi upsert screen (khoá theo `--screen` + `--viewport`) vào `<repo>/.znf/ui-verify/<fp>.json`. Đổi working tree sau khi ghi (vd một fix ở vòng review) làm `fp` lệch, nên artifact cũ không còn khớp lần `check` kế tiếp — buộc phải đo lại.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--repo` | Đường dẫn repo. Mặc định thư mục hiện tại. |
| `--screen` | Tên screen/route đã đo. Bắt buộc. |
| `--screenshot` | Đường dẫn ảnh chụp màn hình đã lưu. Bắt buộc. |
| `--viewport` | Viewport width của lần chụp, 320..3840, phải bằng `innerWidth` trong số đo. Bắt buộc. |
| `--measurement` | JSON đo layout trong trang (innerWidth, child, container, padding, overflowX, overflowY). Bắt buộc. |
| `--fail-reason` | Lỗi nhìn thấy mà số đo không bắt được. Có giá trị thì verdict là `fail`. |

Kit tự tính verdict: `fail` nếu có cạnh tràn hoặc có `--fail-reason`, ngược lại `pass`. Lệnh in `verdict: <pass|fail>`, mỗi cạnh tràn một dòng `overflow: <cạnh> <px>px`, và `failReason: <text>` nếu có.

## Ví dụ

```bash
zenify ui-verify record --repo repos/contact-center-web --screen deal-detail \
  --screenshot /tmp/deal-detail.png \
  --viewport 1920 --measurement "$(cat /tmp/deal-detail.layout.json)"
```
