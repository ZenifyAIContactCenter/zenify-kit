---
title: Dùng credential broker
---

# Dùng credential broker

Khi manifest khai báo `broker:`, bạn không giữ chuỗi kết nối DB, mật khẩu Redis, token Grafana hay key PostHog nào trên máy: kit đổi GitHub token của bạn lấy một JWT sống 15 phút, rồi gọi broker. Trang này là phần việc phía dev. Phía maintainer dựng host xem [Dựng host credential broker](/guides/broker-host).

Manifest không khai báo `broker:` thì mọi lệnh kit chạy như cũ, đọc credential từ `<workspace>/.zenify/.env` ([Credential dev với zenify env](/guides/env)). Manifest nhúng sẵn trong binary khai báo `broker:` cho workspace zenify, nên ở workspace này các lệnh dưới đây đi qua broker.

## Manifest nào quyết định broker

GitHub token của bạn được gửi tới URL broker, nên kit chỉ lấy `broker:` từ hai nguồn: manifest nhúng sẵn trong binary, hoặc file bạn chỉ rõ bằng `--manifest`. Kit không bao giờ đọc `broker:` từ `manifest/repos.yaml` ở thư mục hiện tại, và overlay cá nhân không đổi được `broker:`. Một `--manifest` không tồn tại hoặc sai là lỗi; lệnh không lùi về credential trên máy. Token cache ở `~/.zenify/broker/token.json` ghi kèm URL broker đã cấp, nên token của broker này không bao giờ gửi sang broker khác.

## Cài một lần

1. **Đăng nhập GitHub CLI với scope `read:org`.**

   ```bash
   gh auth login -s read:org
   ```

   Đã đăng nhập từ trước mà thiếu scope thì chạy `gh auth refresh -s read:org`. Broker kiểm tra bạn thuộc org qua GitHub token này; không thuộc org thì broker từ chối và không cấp JWT.
2. **Chạy `zenify doctor`** để kiểm tra broker đã dùng được chưa.

## `zenify doctor` mục `broker`

Mục `broker` kiểm tra lần lượt: `gh` đã đăng nhập, `gh` có scope `read:org`, broker trả lời `/healthz` trong 5 giây, rồi đổi được token. Bước đổi token luôn xin JWT mới, không dùng cache, nên doctor thật sự kiểm tra lại bạn còn trong org. Thành công in `broker ok, login=<login broker cấp token>, role=<lead|dev>`; doctor không bao giờ in token. Khi lỗi, thông báo nói bước nào hỏng:

| Thông báo | Việc cần làm |
|---|---|
| ``gh chưa đăng nhập — chạy `gh auth login` `` | Đăng nhập `gh` như trên |
| ``gh thiếu scope read:org — chạy `gh auth refresh -s read:org` `` | Chạy lệnh được gợi ý |
| `không tới được broker — kiểm tra mạng và broker.url trong manifest rồi chạy lại` | Mở `https://broker.zenify.vn/healthz` trên trình duyệt; không mở được thì báo maintainer |
| `broker từ chối token: <status> not_org_member: <lời nhắn>` | GitHub token chưa thấy bạn trong org: kiểm tra scope `read:org`, và nếu org bật SSO thì authorize token của GitHub CLI cho org trên GitHub, rồi chạy lại |
| `broker từ chối token: 403 not_allowed: …` | Bạn thuộc org nhưng không thuộc team `leaders` hay `zenify-team`; nhờ maintainer thêm vào team đúng |
| `broker từ chối token: 403 login_denied: …` | Login này là tài khoản dùng chung, broker không cấp token; đăng nhập `gh` bằng tài khoản cá nhân |
| `broker từ chối token: <status> rate_limited: …` hoặc `… github_unavailable: …` | Broker hoặc GitHub đang tạm quá tải; đợi một lát rồi chạy lại |
| `broker cấu hình sai, báo người quản trị broker: <lời nhắn>` | Lỗi ở phía host broker, không phải máy bạn; gửi nguyên dòng thông báo cho maintainer |
| `broker từ chối token: <status> <mã khác>: <lời nhắn>` | Hỏi maintainer, kèm nguyên dòng thông báo |

Muốn tự kiểm tra riêng bước đổi token: `zenify broker token` in JWT ra stdout và `login=<login>, role=<role>` ra stderr (`zenify broker headers` cũng in một JWT). Đừng dán kết quả vào chat hay log. Khi broker có, mục `tools` của doctor không đòi `mongosh` và `mysql`.

### Broker từ chối token đang cache

`zenify broker headers` (lệnh Claude Code gọi cho `zenify-grafana` và `zenify-posthog`) dùng JWT đang cache tới khi nó hết hạn, và Claude Code không báo lại cho kit khi broker trả 401. Nếu MCP báo lỗi đăng nhập trong khi `zenify doctor` mục `broker` đã ok, chạy `zenify broker token --refresh` để bỏ cache và xin JWT mới, rồi khởi động lại Claude Code.

## Lead và dev {#lead-dev}

Broker cấp role theo team GitHub trong org: thuộc team `leaders` là `lead`, không thì thuộc team `zenify-team` là `dev`. Org member không thuộc team nào trong hai team bị từ chối (`not_allowed`).

- **Lead** đọc như mọi lệnh ở trang này.
- **Dev** trên staging đọc như lead. Trên production (`--prd`), dev chỉ thấy cấu trúc và plan, không thấy nội dung bản ghi:
  - `db-read`: `collections`, `count` không có `--filter` (tổng số ước lượng qua `estimatedDocumentCount`; `count` có `--filter` chỉ dành cho lead), `explain` ở mức `queryPlanner` (chỉ in plan, không chạy truy vấn nên không có executionStats), `indexes`, `stats`, `tables`, và `sql` dạng `EXPLAIN [FORMAT=…] SELECT …` (đúng một `SELECT`, không subquery, không `ANALYZE`, không `WITH`), `DESCRIBE`/`DESC <bảng>`, `SHOW INDEX`/`SHOW INDEXES`/`SHOW KEYS`, `SHOW CREATE TABLE`, `SHOW TABLE STATUS`, `SHOW TABLES`, `SHOW COLUMNS`. Câu `SHOW`/`DESCRIBE` chỉ gồm từ khoá, tên bảng hoặc database và `LIKE '<chuỗi>'`; có `WHERE`, `(` hay `SELECT` thì bị từ chối. Database audit log (`target=audit`) chỉ dành cho lead.
  - `redis-read`: chỉ `TTL`, `TYPE`, `EXISTS`, `SCAN`, `LLEN`, `SCARD`, `ZCARD`.
  - Truy vấn lỗi chỉ báo `query failed; error detail is withheld for role dev on prd`, vì thông báo lỗi của database có thể trích nội dung bản ghi. Stderr của `mongosh`/`mysql` cũng không được trả về khi truy vấn thành công. Cần xem lỗi chi tiết thì chạy lại trên staging.
  - Giới hạn này là rào chắn để agent không vô tình đọc dữ liệu production, không phải ranh giới chống người cố tình dò dữ liệu: `EXPLAIN` của MySQL vẫn đọc trước dòng khớp khoá chính khi lập plan.
- MCP Grafana, PostHog và tài khoản E2E mở cho cả hai role.

Lệnh vượt quyền của dev báo `lệnh này chỉ dành cho lead (team leaders); dev trên prd chỉ dùng db-read: collections, count (no --filter), explain, indexes, stats, tables, sql: EXPLAIN SELECT without subquery, DESCRIBE <table>, SHOW INDEX|COLUMNS|TABLES|CREATE TABLE|TABLE STATUS; redis-read: EXISTS LLEN SCAN SCARD TTL TYPE ZCARD` và không chạy truy vấn nào. Danh sách này do CLI tự in từ bảng quyền của chính nó, nên khớp với broker cùng phiên bản; broker cũ hơn hoặc mới hơn CLI có thể áp danh sách khác, khi đó nâng cấp cả hai lên cùng phiên bản. Khi token cache (`~/.zenify/broker/token.json`) ghi role `dev`, kit từ chối ngay tại máy, không gọi broker; chưa có cache thì broker quyết định. Đầu mỗi phiên Claude Code, hook SessionStart đọc cache này (không gọi mạng). Với dev nó in một dòng `broker: login=<login> role=dev — prd: …` để agent biết trước giới hạn production. Với mọi role đã có cache, nó in thêm một dòng `data: agent tự chạy zenify data run --stg|--prd [--write] -- <lệnh> (dry-run trước; --prd --write chờ "ok" của người dùng; xem _shared/data-scripts.md)`, dev thêm hậu tố `; prd: cần lead`. Giới hạn chỉ đọc của `db-read` và `redis-read` áp cho mọi người, kể cả lead; việc ghi chỉ đi qua `zenify data run`. `zenify doctor` mục `broker` và `zenify broker token` in role hiện tại; vừa được thêm vào team `leaders` thì chạy `zenify broker token --refresh` để xin JWT mang role mới.

## Đọc database: `db-read`

Cùng lệnh và cùng giới hạn chỉ đọc như [Đọc dữ liệu thật](/guides/read-real-data); khi có broker, truy vấn đi qua broker. Thêm `--prd` để đọc production:

```bash
zenify db-read --prd count chat_rooms
zenify db-read --prd sql 'DESCRIBE agents'
zenify db-read --prd indexes chat_rooms
zenify db-read --prd stats chat_rooms
```

Cờ `--target audit` (chỉ Mongo và chỉ production, chỉ dành cho lead) đọc database audit log thay cho database chính; dùng với `--prd`, ví dụ `zenify db-read --prd --target audit collections`. Cờ `--target lumi` đọc MySQL của lumi-agent (`db_chatbot` trên production, `mass_chatbot_v2` trên staging), chỉ `sql` và `tables`; trên production chỉ dành cho lead, ví dụ `zenify db-read --prd --target lumi sql 'SELECT COUNT(*) FROM goals'`. Riêng với `--target lumi`, dev chỉ bị từ chối trên production, còn staging vẫn mở. `--target audit` không kèm `--prd` hay dùng với truy vấn MySQL cũng bị từ chối. Qua broker, lỗi kết nối không còn gợi ý `.zenify/.env` hay tên biến `MONGO_URL_PRD`, vì bạn không giữ chuỗi kết nối nào.

Kit vẫn kiểm tra tham số ở máy bạn như cũ trước khi gửi lên broker.

## Đọc Redis: `redis-read` {#redis-read}

```bash
zenify redis-read GET some:key
zenify redis-read --prd LRANGE some:list 0 49
```

- Chỉ các lệnh chỉ đọc sau được phép: `GET MGET HGET HGETALL HKEYS LRANGE LLEN SMEMBERS SCARD ZRANGE ZCARD XRANGE TTL TYPE EXISTS SCAN`. Lệnh khác bị từ chối ngay ở máy bạn (mã 2), không gửi lên broker.
- `SCAN` tự thêm `COUNT 100` nếu thiếu; `COUNT` phải trong khoảng 1 đến 1000.
- Broker giới hạn mỗi lần đọc: `LRANGE`, `ZRANGE`, `XRANGE` tối đa 200 phần tử; kết quả tối đa 1 MiB: `GET` và các lệnh khác bị cắt và lệnh ghi thông báo ra stderr, còn `MGET`, `HGET`, `HGETALL`, `SMEMBERS` vượt mức thì bị từ chối; `MGET` tối đa 100 key; `HGETALL`, `HKEYS`, `SMEMBERS` từ chối key có hơn 10000 phần tử.
- Cờ (`--prd`) phải đứng trước lệnh Redis, để tham số như `-1` không bị đọc thành cờ. `--prd` đứng sau lệnh (ví dụ `zenify redis-read GET k --prd`) bị từ chối ngay ở máy bạn với mã 2 (BadArgs), không gửi request nào, vì nếu không `--prd` sẽ thành tham số Redis và lệnh chạy nhầm trên staging.
- Kết quả `LRANGE`, `ZRANGE`, `XRANGE` bị cắt ở 200 phần tử thì lệnh in `zenify redis-read: kết quả đã bị cắt bớt` ra stderr; stdout chỉ chứa dữ liệu. Không có thông báo nghĩa là danh sách đã trả đủ, kể cả khi đúng 200 phần tử.
- Không có broker thì lệnh thoát mã 2 với "redis-read cần broker".

## Chạy script với credential broker: `data run` {#data-run}

```bash
zenify data run --stg -- node docs/scripts/contact-center-be/fix-tags.js --dry-run
zenify data run --stg --write -- node docs/scripts/contact-center-be/fix-tags.js
```

`zenify data run` xin credential từ broker rồi chạy lệnh sau `--` với các biến `MONGO_URL`, `MYSQL_URL`, `REDIS_URL` (thêm `LUMI_MYSQL_URL`, `QDRANT_URL`, `LUMI_APP_URL` khi có `--lumi`). Bạn không giữ chuỗi kết nối nào trong shell. Agent tự chạy lệnh này, không cần bạn gõ `!`.

- Phải chọn đúng một trong `--stg` hoặc `--prd`; không thì thoát mã 2. `--prd` chỉ dành cho lead.
- Lệnh có cả `--prd` và `--write` luôn hiện prompt hỏi bạn trong Claude Code, kể cả khi bật bypass. Lý do: kit thêm hai rule `permissions.ask` phủ cả hai thứ tự cờ, và rule ask thắng rule allow. Ghi staging và dry-run production chạy không hỏi.
- Chạy không có `--write` là dry-run: credential chỉ đọc, lệnh ghi một bản ghi dưới `.zenify/data-runs/`.
- `--write` xin credential ghi và cần một dry-run thành công của đúng lệnh đó (cùng nội dung script) trong 60 phút; bản ghi dùng một lần. Thiếu bản ghi thì lệnh thoát mã 7 trước khi gọi broker và in lệnh dry-run cần chạy trước. Log lần ghi (đã che giá trị) nằm ở `docs/scripts/<repo>/<script>.<UTC>.<env>.write.log` khi script nằm dưới `docs/scripts/<repo>/`, không thì ở `.zenify/data-runs/`.
- Stdout và stderr được che giá trị và host. Exit code của lệnh con là exit code của `zenify`.
- Broker ghi audit mỗi lần xin credential (env, mode, tên biến) và không ghi giá trị. Broker chưa cấu hình secret ghi thì `--write` báo `broker_misconfigured`; nhờ maintainer xem [Secret ghi và lumi](/guides/broker-host#secret-ghi-lumi).

Mục `broker-creds` của `zenify doctor` xin thử credential staging chế độ đọc và in tên các biến broker cấp (không in giá trị); không có broker thì mục này bỏ qua. Lỗi ở đây nghĩa là `zenify data run --stg` cũng sẽ lỗi. Khi `.zenify/.env` còn các key `LUMI_MYSQL_URL_*` hay `QDRANT_URL_*` cũ, mục `env-file` báo `không còn dùng, có thể xoá`: xoá chúng đi, broker cấp thay.

## Tài khoản E2E: `e2e login --role`

```bash
zenify e2e login --port 3327 --role agent
```

Broker cấp một tài khoản E2E riêng cho bạn theo role, trên tenant test của staging. Role hợp lệ: `administrator`, `supervisor`, `team_leader`, `agent`, `sale`, `sale_team_leader`. Lệnh ghi Playwright storage state vào `~/.zenify/playwright/state-<role>.json`; không có `--role` thì dùng `administrator` và ghi `~/.zenify/playwright/state.json` như cũ. Agent `znf:ui-verifier` nạp file này bằng `browser_set_storage_state`. `zenify e2e run` và `zenify visual check` cũng nhận `--role`. Xem [Kiểm thử UI](/workflows/ui-testing).

## Grafana và PostHog cho agent

Khi manifest nhúng sẵn trong binary có `broker:` và lệnh `claude` có trên PATH, `zenify up --apply` đăng ký hai MCP server trong Claude Code: `zenify-grafana` (Loki, Prometheus, Tempo) và `zenify-posthog`. `--manifest` và file manifest ở thư mục hiện tại không bật bước này. Tên `zenify-grafana` và `zenify-posthog` dành riêng cho kit: entry user scope kiểu http dưới tên đó có đường dẫn `/mcp/grafana` hoặc `/mcp/posthog` bị ghi đè bằng URL broker hiện tại (đây là cách kit cập nhật khi URL broker đổi). Server cùng tên với đường dẫn khác, kiểu stdio, hoặc ở project scope thì kit giữ nguyên. Cả hai dùng `type: http` tới broker, và Claude Code lấy header đăng nhập bằng `zenify broker headers` mỗi lần kết nối, nên bạn không điền token nào. Skill `/znf:fix` dùng chúng ở Step 0 để lấy log, trace và latency khi gặp lỗi trên staging hay production.

Sau khi `zenify up --apply`, khởi động lại Claude Code để MCP gắn vào phiên.

## Sandbox của Claude Code

Khi sandbox Bash của Claude Code bật, lệnh chạy trong sandbox chỉ ra mạng qua proxy lọc của Claude Code, nên script nối thẳng tới MongoDB, MySQL hay Redis không kết nối được. Vì vậy `zenify up` thêm `zenify db-read *`, `zenify redis-read *` và `zenify data run *` vào cả `sandbox.excludedCommands` và `permissions.allow`. Lệnh gọi trần (một lệnh, không pipe, không redirect, không `cd`, không `$(…)`) chạy ngoài sandbox, có mạng thẳng và không hỏi quyền. Lệnh ghép, ví dụ có redirect ra file, `cd` hay `$(…)`, vẫn nằm trong sandbox và không nối được, nên hãy chạy lại dạng trần, với script ghi bằng đường dẫn tuyệt đối.

- **Lệnh con của `data run`:** phần sau `--` cũng chạy ngoài sandbox. git-guard vẫn áp luật của nó lên lệnh con. Ghi production vẫn cần role lead ở broker và một lần dry-run trước.
- **Khi nào có hiệu lực:** chỉ sau khi chạy `zenify up` và mở phiên Claude Code mới. Khi sandbox tắt, các mục này không có tác dụng.

## Xem thêm

[`zenify broker token`](/reference/cli/zenify_broker_token), [`zenify broker headers`](/reference/cli/zenify_broker_headers), [`zenify redis-read`](/reference/cli/zenify_redis-read), [`zenify db-read`](/reference/cli/zenify_db-read), [`zenify data run`](/reference/cli/zenify_data_run), [`zenify e2e login`](/reference/cli/zenify_e2e_login), [Đọc dữ liệu thật](/guides/read-real-data), [Dựng host credential broker](/guides/broker-host)
