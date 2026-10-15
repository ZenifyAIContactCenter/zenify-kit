---
title: Credential dev với zenify env
---

# Credential dev với `zenify env`

Credential cho tool dev và agent (chuỗi kết nối DB, tài khoản đăng nhập test) nằm trong một file riêng của workspace, không nằm trong khối `env` của Claude settings. [`zenify env`](/reference/cli/zenify_env) quản lý file đó.

## File nằm ở đâu

File là `<workspace>/.zenify/.env`, quyền 0600. Nó không phải `.env` của một repo: các app không đọc nó, chỉ `zenify db-read`, `zenify e2e`, `zenify visual` và `zenify env exec` đọc. Khi một biến đã có sẵn trong môi trường của tiến trình thì giá trị đó thắng giá trị trong file, trừ khi giá trị đó rỗng.

Sau `zenify up --apply`, `~/.claude/settings.json` có luật deny `Read(//<workspace>/.zenify/.env)` với `<workspace>` là đường dẫn tuyệt đối, nên luật có hiệu lực ở mọi phiên, kể cả phiên mở trong một repo hay worktree. Luật này chặn agent vô tình đọc file: nó chặn tool Read và các lệnh như `cat`, `head`, `sed` trong Bash, nhưng không chặn một script tự mở file. Bạn vẫn tự đọc, sửa file bình thường.

## Khung file

`zenify up --apply` dựng `.zenify/.env` như một file cấu hình: mọi key của manifest luôn có mặt, key chưa có giá trị là dòng `KEY=` rỗng. Mỗi key có hai dòng comment ngay phía trên: dòng 1 là mô tả; dòng 2 là `Lấy: <howTo> · <phạm vi> · secret hoặc không secret · <repo>`, trong đó `Lấy:` chỉ hiện khi manifest có `howTo`, và `<repo>` chỉ hiện với key khai trong một repo (như `lumi-agent`). Các key xếp theo bốn mục, đúng thứ tự: Staging, Tooling, Production, rồi Custom. Ví dụ rút gọn (file thật liệt kê đủ mọi key của manifest):

```bash
# zenify-kit — credential dev. Kit giữ khung; bạn chỉ điền giá trị. Key trống = chưa cấu hình.

# ══ Staging ══
# Chuỗi kết nối MongoDB cho zenify db-read
# shared · secret
MONGO_URL=

# URL MySQL cho zenify db-read (mysql://user:pass@host:port/database)
# shared · secret
MYSQL_URL=

# ══ Tooling ══
# Tên miền (tenant) đăng nhập cho zenify e2e login
# shared · không secret
E2E_DOMAIN=

# ══ Production ══
# MongoDB production, DB 3csoft
# personal · secret
MONGO_URL_PRD=

# ══ Custom (ngoài manifest) ══
# key cá nhân
OPENROUTER_API_KEY='...'
```

Quy tắc khi `zenify up --apply` dựng lại file:

- Giá trị bạn đã điền được giữ nguyên; kit chỉ viết lại phần khung quanh nó.
- Kit coi mọi khối comment liền nhau nằm ngay trên một key manifest là của nó và thay bằng hai dòng sinh từ manifest. Vì vậy comment bạn tự viết ngay trên một key manifest bị bỏ, và mô tả đổi trong manifest được cập nhật ở lần chạy sau. Muốn ghi chú cho key manifest, đổi mô tả trong manifest.
- Dòng comment không gắn với key nào, và dòng gán bị comment như `# OLD_TOKEN=...` (có thể còn giữ giá trị cũ), được giữ lại ở cuối mục Custom.
- Key ngoài manifest nằm ở mục Custom. Dòng comment ngay trên nó là mô tả của key đó.
- Một dòng trống hoặc `KEY=` rỗng nghĩa là chưa đặt: biến môi trường của tiến trình thắng, và `zenify env list` báo `empty`.
- Một key gán nhiều lần thì giá trị dùng là lần gán cuối cùng khác rỗng; mỗi giá trị khác rỗng còn lại được giữ thành dòng comment `# KEY=...` ở cuối mục Custom, và `zenify up` báo `env: giữ giá trị trùng của <tên> dưới dạng comment`.
- Chạy lại `zenify up --apply` trên file đã dựng thì file không đổi.

## Key do manifest khai báo

Manifest `manifest/repos.yaml` khai từng key gồm tên, mô tả, phạm vi (`shared` hoặc `personal`), cờ `secret` và nhóm (`group`, mặc định `staging`). Giá trị không bao giờ nằm trong manifest.

| Key | Dùng cho | Phạm vi | Nhóm |
|---|---|---|---|
| `MONGO_URL` | `zenify db-read` đọc MongoDB | shared | staging |
| `MYSQL_URL` | `zenify db-read sql`, `zenify db-read tables` | shared | staging |
| `E2E_DOMAIN` | tenant đăng nhập của `zenify e2e login` | shared, không phải secret | tooling |
| `E2E_EMAIL`, `E2E_PASSWORD` | tài khoản đăng nhập của `zenify e2e login` | personal | tooling |
| `POSTHOG_API_KEY` | key PostHog của bạn | personal | tooling |
| `MONGO_URL_PRD` | MongoDB production, DB 3csoft | personal | production |
| `AUDIT_MONGO_URL_PRD` | MongoDB `audit_logs` production | personal | production |
| `REDIS_URL_PRD` | Redis production | personal | production |
| `LUMI_MYSQL_URL_STG`, `QDRANT_URL_STG`, `LUMI_APP_URL_STG` | MySQL, Qdrant và URL ứng dụng của `lumi-agent` trên staging | shared; `LUMI_APP_URL_STG` không phải secret | staging |
| `LUMI_MYSQL_URL_PRD`, `QDRANT_URL_PRD` | MySQL và Qdrant của `lumi-agent` trên production | shared | production |

Key của `lumi-agent` có mặt trong `.zenify/.env` của mọi workspace, kể cả workspace không clone repo đó; chỉ người làm trên `lumi-agent` mới điền.

Bảng này chỉ mô tả kit dùng key để làm gì. Ai cấp key, địa chỉ cụ thể và cách lấy không thuộc về kit, hỏi trong team.

Wizard `zenify up` và form `zenify env` chỉ hỏi các key toàn cục trừ nhóm production: `MONGO_URL`, `MYSQL_URL`, `E2E_*`, `POSTHOG_API_KEY`. Key production, và key của một repo, bạn điền bằng `zenify env set`, chọn trong form `zenify env` hoặc sửa file. `zenify doctor` cũng bỏ qua key production khi báo key nào trống.

## `MYSQL_URL`

`MYSQL_URL` gom kết nối MySQL vào một dòng, dạng `mysql://user:pass@host:port/database`. Thiếu `:port` thì dùng cổng 3306. User, host và database là bắt buộc; URL sai dạng làm `zenify db-read sql` báo lỗi nêu tên `MYSQL_URL` và dạng đúng, không in giá trị.

Ký tự `@ : / # ? %` và dấu cách trong user hoặc mật khẩu phải percent-encode: `@` thành `%40`, `:` thành `%3A`, `/` thành `%2F`, `#` thành `%23`, `?` thành `%3F`, `%` thành `%25`, dấu cách thành `%20`. Ví dụ mật khẩu `p@ss` viết là `p%40ss`. `zenify db-read` giải mã trước khi gọi `mysql`, và mật khẩu chỉ đi qua biến `MYSQL_PWD` của tiến trình con, không nằm trên dòng lệnh. Nhập bằng `zenify env set MYSQL_URL` để giá trị không hiện trên màn hình.

Bản kit trước dùng năm key `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_DATABASE`. `zenify up --apply` tự ghép chúng thành `MYSQL_URL`, xem [Chuyển từ khối `env` của settings](#chuyen-tu-khoi-env-cua-settings).

## Xem và nhập key

```bash
zenify env list
zenify env
zenify env set OPENROUTER_API_KEY --desc "key cá nhân"
zenify env edit
```

`zenify env list` in tên, `set` hoặc `empty`, nguồn `manifest` hoặc `custom`, mô tả; chỉ key có `secret: false` mới in giá trị. `zenify env` không đối số mở form nhập ẩn. `zenify env set` cần terminal thật, nhận giá trị ẩn. `zenify env edit` mở file bằng `$EDITOR`.

## Key custom

Key chỉ bạn cần, không có trong manifest: `zenify env set KEY --desc "mô tả"`. `--desc` là bắt buộc, trừ khi file đã có sẵn dòng mô tả của key đó; thiếu thì lệnh báo lỗi. Lệnh cũng từ chối ghi khi giá trị nhập vào trống và giá trị hiện tại cũng trống. Key đó là `custom`, vào mục Custom của file, coi như secret nên không bao giờ bị in ra. Mô tả là dòng comment ngay trên key, để agent đọc được key dùng làm gì. Key custom thiếu mô tả thì `zenify up` (xem trước) báo `env: custom thiếu mô tả <tên>` và `zenify doctor` báo mục `env-file`.

## Thêm key cho cả team

Mở PR vào `manifest/repos.yaml`, thêm vào `secretKeys` (của manifest hoặc của repo cần nó) một mục đủ sáu trường:

```yaml
- name: SOME_KEY
  description: "Dùng để làm gì"
  howTo: "Ai hoặc ở đâu cấp key"
  scope: personal
  secret: true
  group: staging
```

`group` là `staging` (mặc định khi bỏ trống), `tooling` hoặc `production`; nó quyết định key nằm ở mục nào trong file và wizard có hỏi không (nhóm production thì không hỏi).

Không bao giờ ghi giá trị vào manifest.

## Chạy script tay

```bash
zenify env exec -- node scripts/check.js
```

Lệnh con nhận env hiện có cộng key trong file, mà không cần export vào shell. Stdout và stderr được che theo dòng: giá trị của key secret (key manifest có `secret: true` và mọi key `custom`) dài từ 6 ký tự trở lên bị thay bằng `***`; giá trị của key có `secret: false` hoặc ngắn hơn 6 ký tự không bị che. Exit code của lệnh con là exit code của `zenify`.

`zenify env exec` dành cho lệnh không tương tác. Output chỉ hiện khi có trọn một dòng, nên một lời nhắc nhập liệu không kết thúc bằng xuống dòng sẽ không hiện ra.

## Key bị bỏ khỏi manifest

Khi một PR gỡ key khỏi manifest, `zenify up --apply` xoá dòng key đó (cùng comment kit sinh) khỏi `.zenify/.env`, nhưng chỉ với key mà kit đã từng ghi nhận; key bạn tự thêm vào mục Custom không bao giờ bị xoá. Kit chỉ xoá khi có đủ hai điều kiện:

- Manifest của lần chạy là bản nhúng trong binary, không phải `--manifest` hay `manifest/repos.yaml` dưới thư mục hiện tại.
- Binary đang chạy không cũ hơn binary đã ghi nhận danh sách key lần trước. Bản build `dev` chỉ so được với bản `dev`.

Thiếu một trong hai, kit không xoá gì: key vẫn nằm trong file (ở mục Custom, giữ dòng mô tả) và vẫn được ghi nhận, để một lần chạy sau đủ điều kiện xoá nó. Kit báo `env: giữ <tên> — manifest không phải bản nhúng (<nguồn>)` hoặc `env: giữ <tên> — binary cũ hơn bản đã ghi`.

`zenify up` không có `--apply` xem trước thay đổi, chỉ in tên key; wizard `zenify up` hiện cùng các dòng này trên màn hình kế hoạch, trước khi hỏi xác nhận:

```
env: thêm MONGO_URL_PRD, AUDIT_MONGO_URL_PRD, REDIS_URL_PRD
env: xoá OLD_KEY
env: custom thiếu mô tả OPENROUTER_API_KEY
```

Muốn giữ giá trị của một key sắp bị xoá, chép nó sang tên khác hoặc sang nơi khác trước khi chạy `zenify up --apply`.

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
- Giá trị không ghép được thành URL hợp lệ, ví dụ `MYSQL_PORT` không phải số hoặc `MYSQL_HOST` có kèm cổng: kit giữ nguyên file và báo `env: bỏ qua MYSQL_URL — MYSQL_* không ghép được thành URL hợp lệ, xử lý tay`.
- Một key `MYSQL_*` cũ còn bị giữ lại trong settings vì xung đột hoặc không phải chuỗi một dòng: kit không ghép, để bạn xử lý xung đột trước, và báo `env: bỏ qua MYSQL_URL — MYSQL_* còn xung đột trong settings, xử lý tay`.

`zenify doctor` có ba mục liên quan: `secrets` báo theo tên key toàn cục (trừ nhóm production) nào có giá trị, key nào trống; `settings-env` báo theo tên những key còn nằm trong settings, kèm hướng dẫn chạy `zenify up --apply` hoặc xử lý tay với key mà migration bỏ qua; `env-file` báo file có quyền rộng hơn 0600 (trừ trên Windows), key `custom` chưa có mô tả, và các key `MYSQL_*` cũ còn nằm trong `.zenify/.env` chưa được ghép thành `MYSQL_URL`. `zenify down --apply` gỡ luật deny `.zenify/.env` của workspace đó khỏi `~/.claude/settings.json`, giữ nguyên luật bạn tự thêm và luật của workspace khác.

## Khôi phục khi hạ bản kit

Bản kit cũ đọc credential từ khối `env` của `settings.local.json`. Nếu hạ bản: chạy `zenify env edit`, chép tay các key cần dùng về khối `env` của `<workspace>/.claude/settings.local.json`. Bản trước `v0.36.0` không đọc `MYSQL_URL`: tách nó lại thành năm key `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_DATABASE`, với user và mật khẩu đã giải mã percent-encoding.

## Xem thêm

[`zenify env`](/reference/cli/zenify_env), [Đọc dữ liệu thật](/guides/read-real-data), [Kiểm thử UI](/workflows/ui-testing), [Onboard workspace và repo](/guides/onboard-workspace)

<!-- Nguồn (cho người bảo trì, không hiển thị):
- reference/cli/zenify_env*.md (generated)
- manifest/repos.yaml (secretKeys), internal/devenv/render.go (khung file), internal/cli/up.go (renderEnvStep), internal/cli/env.go, internal/dbread/dbread.go (parseMySQLURL), internal/apply/mysqlurl.go (MergeMySQLURL), internal/cli/doctor_checks.go (env-file, settings-env), internal/cli/doctor_checks.go (secrets), internal/apply/permissions.go (deny rule), internal/devenv/mask.go (exec masking)
-->
