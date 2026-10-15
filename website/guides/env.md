---
title: Credential dev với zenify env
---

# Credential dev với `zenify env`

Credential cho tool dev và agent (chuỗi kết nối DB, tài khoản đăng nhập test) nằm trong một file riêng của workspace, không nằm trong khối `env` của Claude settings. [`zenify env`](/reference/cli/zenify_env) quản lý file đó.

## File nằm ở đâu

File là `<workspace>/.zenify/.env`, quyền 0600. Nó không phải `.env` của một repo: các app không đọc nó, chỉ `zenify db-read`, `zenify e2e`, `zenify visual` và `zenify env exec` đọc. Khi một biến đã có sẵn trong môi trường của tiến trình thì giá trị đó thắng giá trị trong file, trừ khi giá trị đó rỗng.

Sau `zenify up --apply`, `~/.claude/settings.json` có luật deny `Read(//<workspace>/.zenify/.env)` với `<workspace>` là đường dẫn tuyệt đối, nên luật có hiệu lực ở mọi phiên, kể cả phiên mở trong một repo hay worktree. Luật này chặn agent vô tình đọc file: nó chặn tool Read và các lệnh như `cat`, `head`, `sed` trong Bash, nhưng không chặn một script tự mở file. Bạn vẫn tự đọc, sửa file bình thường.

## Key do manifest khai báo

Manifest `manifest/repos.yaml` khai từng key gồm tên, mô tả, phạm vi (`shared` hoặc `personal`) và cờ `secret`. Giá trị không bao giờ nằm trong manifest.

| Key | Dùng cho | Phạm vi |
|---|---|---|
| `MONGO_URL` | `zenify db-read` đọc MongoDB | shared |
| `MYSQL_URL` | `zenify db-read sql`, `zenify db-read tables` | shared |
| `E2E_DOMAIN` | tenant đăng nhập của `zenify e2e login` | shared, không phải secret |
| `E2E_EMAIL`, `E2E_PASSWORD` | tài khoản đăng nhập của `zenify e2e login` | personal |
| `POSTHOG_API_KEY` | key PostHog của bạn | personal |
| `LUMI_MYSQL_URL_STG`, `LUMI_MYSQL_URL_PRD`, `QDRANT_URL_STG`, `QDRANT_URL_PRD`, `LUMI_APP_URL_STG`, `OPENROUTER_API_KEY` | chỉ khi workspace có `lumi-agent` | theo manifest |

Bảng này chỉ mô tả kit dùng key để làm gì. Ai cấp key, địa chỉ cụ thể và cách lấy không thuộc về kit, hỏi trong team.

Wizard `zenify up` chỉ hỏi các key toàn cục: `MONGO_URL`, `MYSQL_URL`, `E2E_*`, `POSTHOG_API_KEY`. Key của một repo như `OPENROUTER_API_KEY` thêm sau, bằng `zenify env set`, `zenify env` hoặc sửa file.

## `MYSQL_URL`

`MYSQL_URL` gom kết nối MySQL vào một dòng, dạng `mysql://user:pass@host:port/database`. Thiếu `:port` thì dùng cổng 3306. User, host và database là bắt buộc; URL sai dạng làm `zenify db-read sql` báo lỗi nêu tên `MYSQL_URL` và dạng đúng, không in giá trị.

Ký tự `@ : / # ?` trong user hoặc mật khẩu phải percent-encode: `@` thành `%40`, `:` thành `%3A`, `/` thành `%2F`, `#` thành `%23`, `?` thành `%3F`. Ví dụ mật khẩu `p@ss` viết là `p%40ss`. `zenify db-read` giải mã trước khi gọi `mysql`, và mật khẩu chỉ đi qua biến `MYSQL_PWD` của tiến trình con, không nằm trên dòng lệnh. Nhập bằng `zenify env set MYSQL_URL` để giá trị không hiện trên màn hình.

Bản kit trước dùng năm key `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_DATABASE`. `zenify up --apply` tự ghép chúng thành `MYSQL_URL`, xem [Chuyển từ khối `env` của settings](#chuyen-tu-khoi-env-cua-settings).

## Xem và nhập key

```bash
zenify env list
zenify env
zenify env set OPENROUTER_API_KEY --desc "key cá nhân"
zenify env edit
```

`zenify env list` in tên, `set` hoặc `empty`, nguồn `manifest` hoặc `local`, mô tả; chỉ key có `secret: false` mới in giá trị. `zenify env` không đối số mở form nhập ẩn. `zenify env set` cần terminal thật, nhận giá trị ẩn. `zenify env edit` mở file bằng `$EDITOR`.

## Thêm key riêng

Key chỉ bạn cần: `zenify env set KEY --desc "mô tả"`. Key đó là `local`, coi như secret nên không bao giờ bị in ra. Thiếu dòng mô tả thì `zenify doctor` báo mục `env-file`.

## Thêm key cho cả team

Mở PR vào `manifest/repos.yaml`, thêm vào `secretKeys` (của manifest hoặc của repo cần nó) một mục đủ năm trường:

```yaml
- name: SOME_KEY
  description: "Dùng để làm gì"
  howTo: "Ai hoặc ở đâu cấp key"
  scope: personal
  secret: true
```

Không bao giờ ghi giá trị vào manifest.

## Chạy script tay

```bash
zenify env exec -- node scripts/check.js
```

Lệnh con nhận env hiện có cộng key trong file, mà không cần export vào shell. Stdout và stderr được che theo dòng: giá trị của key secret (key manifest có `secret: true` và mọi key `local`) dài từ 6 ký tự trở lên bị thay bằng `***`; giá trị của key có `secret: false` hoặc ngắn hơn 6 ký tự không bị che. Exit code của lệnh con là exit code của `zenify`.

`zenify env exec` dành cho lệnh không tương tác. Output chỉ hiện khi có trọn một dòng, nên một lời nhắc nhập liệu không kết thúc bằng xuống dòng sẽ không hiện ra.

## Chuyển từ khối `env` của settings

`zenify up --apply` chuyển các key của manifest từ khối `env` trong `.claude/settings.local.json` (workspace, từng repo và từng worktree của repo dưới `.worktrees/`) sang `.zenify/.env`. Quy tắc:

- Giá trị giống nhau giữa các nơi: ghi vào `.zenify/.env` và xoá khỏi settings.
- Hai file settings, hoặc settings và `.zenify/.env`, có giá trị khác nhau cho cùng một key: kit để nguyên mọi nơi và báo `env: bỏ qua <tên> — hai giá trị khác nhau, xử lý tay`. Bạn chọn giá trị đúng rồi tự xoá bản còn lại.
- Giá trị không phải chuỗi, hoặc chuỗi có xuống dòng: kit để nguyên và báo `env: bỏ qua <tên> — giá trị không phải chuỗi một dòng, xử lý tay`.
- File settings của một worktree không đọc được (JSON hỏng, hoặc `env` không phải object): kit bỏ qua file đó, báo `env: bỏ qua <đường dẫn> — file settings của worktree không đọc được, xử lý tay` và vẫn chuyển các file còn lại. File settings của workspace hoặc của repo hỏng thì migration dừng.
- Chỉ key của manifest được chuyển, cùng năm key MySQL cũ `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_DATABASE`; key khác trong khối `env` giữ nguyên.
- Output chỉ in tên key, không in giá trị.

Sau bước chuyển, `zenify up --apply` ghép năm key MySQL cũ trong `.zenify/.env` thành `MYSQL_URL`:

- Có đủ `MYSQL_HOST`, `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_DATABASE` khác rỗng (`MYSQL_PORT` không bắt buộc) và chưa có `MYSQL_URL`: kit ghi `MYSQL_URL` với user và mật khẩu đã percent-encode, rồi xoá năm dòng cũ cùng dòng mô tả của chúng.
- Đã có `MYSQL_URL` trùng với giá trị ghép: kit chỉ xoá năm dòng cũ.
- Đã có `MYSQL_URL` khác giá trị ghép: kit giữ nguyên tất cả và báo `env: bỏ qua MYSQL_URL — khác giá trị ghép từ MYSQL_*, xử lý tay`. Bạn giữ giá trị đúng bằng `zenify env set MYSQL_URL`, rồi xoá năm dòng cũ bằng `zenify env edit`.
- Thiếu một key bắt buộc: kit không ghép, giữ nguyên file và báo tên key thiếu.

`zenify doctor` có ba mục liên quan: `secrets` báo theo tên key toàn cục nào có giá trị, key nào trống; `settings-env` báo theo tên những key còn nằm trong settings, kèm hướng dẫn chạy `zenify up --apply` hoặc xử lý tay với key mà migration bỏ qua; `env-file` báo file có quyền rộng hơn 0600 (trừ trên Windows) và key `local` chưa có mô tả. `zenify down --apply` gỡ luật deny `.zenify/.env` của workspace đó khỏi `~/.claude/settings.json`, giữ nguyên luật bạn tự thêm và luật của workspace khác.

## Khôi phục khi hạ bản kit

Bản kit cũ đọc credential từ khối `env` của `settings.local.json`. Nếu hạ bản: chạy `zenify env edit`, chép tay các key cần dùng về khối `env` của `<workspace>/.claude/settings.local.json`. Bản trước `v0.36.0` không đọc `MYSQL_URL`: tách nó lại thành năm key `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_DATABASE`, với user và mật khẩu đã giải mã percent-encoding.

## Xem thêm

[`zenify env`](/reference/cli/zenify_env), [Đọc dữ liệu thật](/guides/read-real-data), [Kiểm thử UI](/workflows/ui-testing), [Onboard workspace và repo](/guides/onboard-workspace)

<!-- Nguồn (cho người bảo trì, không hiển thị):
- reference/cli/zenify_env*.md (generated)
- manifest/repos.yaml (secretKeys), internal/dbread/dbread.go (parseMySQLURL), internal/apply/mysqlurl.go (MergeMySQLURL), internal/cli/doctor_checks.go (env-file, settings-env), internal/cli/doctor_checks.go (secrets), internal/apply/permissions.go (deny rule), internal/devenv/mask.go (exec masking)
-->
