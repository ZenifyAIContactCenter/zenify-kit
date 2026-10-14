---
title: Credential dev với zenify env
---

# Credential dev với `zenify env`

Credential cho tool dev và agent (chuỗi kết nối DB, tài khoản đăng nhập test) nằm trong một file riêng của workspace, không nằm trong khối `env` của Claude settings. [`zenify env`](/reference/cli/zenify_env) quản lý file đó.

## File nằm ở đâu

File là `<workspace>/.zenify/.env`, quyền 0600. Nó không phải `.env` của một repo: các app không đọc nó, chỉ `zenify db-read`, `zenify e2e`, `zenify visual` và `zenify env exec` đọc. Khi một biến đã có sẵn trong môi trường của tiến trình thì giá trị đó thắng giá trị trong file, trừ khi giá trị đó rỗng.

Sau `zenify up --apply`, `<workspace>/.claude/settings.json` có luật deny `Read(/.zenify/.env)`, nên agent không đọc được file này. Bạn vẫn tự đọc, sửa file bình thường.

## Key do manifest khai báo

Manifest `manifest/repos.yaml` khai từng key gồm tên, mô tả, phạm vi (`shared` hoặc `personal`) và cờ `secret`. Giá trị không bao giờ nằm trong manifest.

| Key | Dùng cho | Phạm vi |
|---|---|---|
| `MONGO_URL` | `zenify db-read` đọc MongoDB | shared |
| `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_DATABASE` | `zenify db-read sql` | shared, không phải secret |
| `MYSQL_USER`, `MYSQL_PASSWORD` | `zenify db-read sql` | shared |
| `E2E_DOMAIN` | tenant đăng nhập của `zenify e2e login` | shared, không phải secret |
| `E2E_EMAIL`, `E2E_PASSWORD` | tài khoản đăng nhập của `zenify e2e login` | personal |
| `POSTHOG_API_KEY` | key PostHog của bạn | personal |
| `LUMI_MYSQL_URL_STG`, `LUMI_MYSQL_URL_PRD`, `QDRANT_URL_STG`, `QDRANT_URL_PRD`, `LUMI_APP_URL_STG`, `OPENROUTER_API_KEY` | chỉ khi workspace có `lumi-agent` | theo manifest |

Bảng này chỉ mô tả kit dùng key để làm gì. Ai cấp key, địa chỉ cụ thể và cách lấy không thuộc về kit, hỏi trong team.

Wizard `zenify up` chỉ hỏi các key toàn cục: `MONGO_URL`, `MYSQL_*`, `E2E_*`, `POSTHOG_API_KEY`. Key của một repo như `OPENROUTER_API_KEY` thêm sau, bằng `zenify env set`, `zenify env` hoặc sửa file.

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

Lệnh con nhận env hiện có cộng key trong file, mà không cần export vào shell. Stdout và stderr được che theo dòng: mọi giá trị trong file dài từ 6 ký tự trở lên bị thay bằng `***`. Exit code của lệnh con là exit code của `zenify`.

## Chuyển từ khối `env` của settings

`zenify up --apply` chuyển các key của manifest từ khối `env` trong `.claude/settings.local.json` (workspace và từng repo) sang `.zenify/.env`. Quy tắc:

- Giá trị giống nhau giữa các nơi: ghi vào `.zenify/.env` và xoá khỏi settings.
- Hai file settings, hoặc settings và `.zenify/.env`, có giá trị khác nhau cho cùng một key: kit để nguyên mọi nơi và báo `env: bỏ qua <tên> — hai giá trị khác nhau, xử lý tay`. Bạn chọn giá trị đúng rồi tự xoá bản còn lại.
- Chỉ key của manifest được chuyển; key khác trong khối `env` giữ nguyên.
- Output chỉ in tên key, không in giá trị.

`zenify doctor` có hai mục liên quan: `settings-env` báo theo tên những key còn nằm trong settings, `env-file` báo file có quyền rộng hơn 0600 và key `local` chưa có mô tả. `zenify down --apply` gỡ luật `Read(/.zenify/.env)` do kit thêm và giữ nguyên luật bạn tự thêm.

## Khôi phục khi hạ bản kit

Bản kit cũ đọc credential từ khối `env` của `settings.local.json`. Nếu hạ bản: chạy `zenify env edit`, chép tay các key cần dùng về khối `env` của `<workspace>/.claude/settings.local.json`.

## Xem thêm

[`zenify env`](/reference/cli/zenify_env), [Đọc dữ liệu thật](/guides/read-real-data), [Kiểm thử UI](/workflows/ui-testing), [Onboard workspace và repo](/guides/onboard-workspace)

<!-- Nguồn (cho người bảo trì, không hiển thị):
- reference/cli/zenify_env*.md (generated)
- manifest/repos.yaml (secretKeys), internal/cli/doctor_checks.go (env-file, settings-env), internal/apply/permissions.go (deny rule)
-->
