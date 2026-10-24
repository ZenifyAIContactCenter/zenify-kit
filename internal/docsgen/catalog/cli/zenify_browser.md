---
summary: Điều khiển trình duyệt qua playwright-cli đã ghim, mỗi phiên một trình duyệt và một thư mục riêng, để `znf:ui-verifier` chạy song song.
---
## Khi nào dùng

Khi `znf:ui-verifier` (hoặc bạn) cần lái một web app đang chạy: mở trang, bấm, đo, chụp ảnh. Lệnh chuyển nguyên các tham số cho `@playwright/cli` 0.1.22 mà `zenify up` đã cài vào `~/.zenify/playwright/cli/pkg`; chưa cài thì lệnh báo "playwright-cli chưa cài, chạy `zenify up` trước" và thoát mã 1.

Cú pháp bắt buộc có tên phiên: `zenify browser -s=<phiên> <lệnh> [args…]`. Cũng nhận `-s <phiên>`, `--session=<phiên>` và `--session <phiên>`. Tên phiên chỉ gồm chữ, số, `_` và `-`. Mỗi phiên có trình duyệt riêng, nên nhiều phiên chạy song song được; dùng phiên nào thì chỉ đụng phiên đó.

Trước lệnh con chỉ nhận thêm `--raw` và `--json`; cờ khác đứng trước lệnh con bị từ chối vì nó có thể nuốt giá trị và che mất lệnh con. Ba lệnh con bị chặn vì ảnh hưởng mọi phiên hoặc bản cài chung: `close-all`, `kill-all` và `install`.

## Kết quả

Lệnh chạy từ thư mục phiên `~/.zenify/playwright/sessions/<phiên>/` (tự tạo), nên mọi file CLI ghi (snapshot, console log, ảnh chụp) nằm ở `~/.zenify/playwright/sessions/<phiên>/.playwright-cli/`, ngoài repo. Lệnh viết lại mọi đường dẫn tương đối `.playwright-cli/…` trong output thành đường dẫn tuyệt đối trong thư mục đó, để agent đọc thẳng file mà không phụ thuộc cwd. Biến `E2E_DOMAIN`, `E2E_EMAIL`, `E2E_PASSWORD` không được chuyển cho tiến trình con.

Mã thoát: 0 khi CLI thành công; mã của CLI khi nó thất bại; 2 khi thiếu hoặc sai tên phiên, cờ không được phép trước lệnh con, hoặc lệnh con bị chặn; 1 khi CLI chưa cài hoặc không tạo được thư mục phiên.

## Ví dụ

```bash
zenify browser -s=uiv-1 open
zenify browser -s=uiv-1 state-load ~/.zenify/playwright/state.json
zenify browser -s=uiv-1 goto http://localhost:3327
zenify browser --raw -s=uiv-1 eval "location.href"
zenify browser -s=uiv-1 screenshot
zenify browser -s=uiv-1 close
```
