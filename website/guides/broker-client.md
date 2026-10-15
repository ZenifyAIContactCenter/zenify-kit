---
title: Dùng credential broker
---

# Dùng credential broker

Workspace zenify khai báo `broker:` trong manifest (`https://broker.zenify.vn`). Nhờ đó bạn không giữ chuỗi kết nối DB, mật khẩu Redis, token Grafana hay key PostHog nào trên máy: kit đổi GitHub token của bạn lấy một JWT sống 15 phút, rồi gọi broker. Trang này là phần việc phía dev. Phía maintainer dựng host xem [Dựng host credential broker](/guides/broker-host).

Manifest không khai báo `broker:` thì mọi lệnh kit chạy như cũ, đọc credential từ `<workspace>/.zenify/.env` ([Credential dev với zenify env](/guides/env)).

## Cài một lần

1. **Cài Cloudflare WARP và vào team.** Cài ứng dụng Cloudflare WARP, đăng nhập bằng GitHub theo policy của org `ZenifyAIContactCenter` rồi bật WARP. Broker chỉ tới được khi WARP đang bật.
2. **Đăng nhập GitHub CLI với scope `read:org`.**

   ```bash
   gh auth login -s read:org
   ```

   Đã đăng nhập từ trước mà thiếu scope thì chạy `gh auth refresh -s read:org`. Broker kiểm tra bạn thuộc org qua GitHub token này; không thuộc org thì broker từ chối và không cấp JWT.
3. **Chạy `zenify doctor`** để kiểm tra cả ba điều kiện cùng lúc.

## `zenify doctor` mục `broker`

Mục `broker` kiểm tra lần lượt: `gh` đã đăng nhập, `gh` có scope `read:org`, broker trả lời `/healthz` trong 5 giây, rồi đổi được token. Thành công in `broker ok, login=<tên GitHub>`; doctor không bao giờ in token. Khi lỗi, thông báo nói bước nào hỏng:

| Thông báo | Việc cần làm |
|---|---|
| `gh chưa đăng nhập — chạy gh auth login` | Đăng nhập `gh` như trên |
| `gh thiếu scope read:org — chạy gh auth refresh -s read:org` | Chạy lệnh được gợi ý |
| `không tới được broker — bật Cloudflare WARP rồi chạy lại` | Bật WARP, kiểm tra WARP đã vào đúng team, chạy lại |
| `broker từ chối token: <mã> <lý do>` | GitHub login của bạn không được broker chấp nhận; hỏi maintainer |

Muốn tự kiểm tra riêng bước đổi token: `zenify broker token` in JWT ra stdout. Đừng dán kết quả vào chat hay log.

## Đọc database: `db-read`

Cùng lệnh và cùng giới hạn chỉ đọc như [Đọc dữ liệu thật](/guides/read-real-data); khi có broker, truy vấn đi qua broker. Thêm `--prd` để đọc production:

```bash
zenify db-read --prd count chat_rooms
zenify db-read --prd sql 'DESCRIBE agents'
```

Kit vẫn kiểm tra tham số ở máy bạn như cũ trước khi gửi lên broker.

## Đọc Redis: `redis-read` {#redis-read}

```bash
zenify redis-read GET some:key
zenify redis-read --prd LRANGE some:list 0 -1
```

- Chỉ các lệnh chỉ đọc sau được phép: `GET MGET HGET HGETALL HKEYS LRANGE LLEN SMEMBERS SCARD ZRANGE ZCARD XRANGE TTL TYPE EXISTS SCAN`. Lệnh khác bị từ chối ngay ở máy bạn (mã 2), không gửi lên broker.
- `SCAN` tự thêm `COUNT 100` nếu thiếu; `COUNT` phải trong khoảng 1 đến 1000.
- Kết quả quá lớn bị broker cắt bớt, và lệnh ghi thông báo ra stderr.
- Cờ (`--prd`) phải đứng trước lệnh Redis, để tham số như `-1` không bị đọc thành cờ.
- Không có broker thì lệnh thoát mã 2 với "redis-read cần broker".

## Tài khoản E2E: `e2e login --role`

```bash
zenify e2e login --port 3327 --role agent
```

Broker cấp một tài khoản E2E riêng cho bạn theo role, trên tenant test của staging. Role hợp lệ: `administrator`, `supervisor`, `team_leader`, `agent`, `sale`, `sale_team_leader`. Lệnh ghi Playwright storage state vào `~/.zenify/playwright/state-<role>.json`; không có `--role` thì dùng `administrator` và ghi `~/.zenify/playwright/state.json` như cũ. Agent `znf:ui-verifier` nạp file này bằng `browser_set_storage_state`. `zenify e2e run` và `zenify visual check` cũng nhận `--role`. Xem [Kiểm thử UI](/workflows/ui-testing).

## Grafana và PostHog cho agent

Khi manifest có `broker:`, `zenify up --apply` đăng ký hai MCP server trong Claude Code: `zenify-grafana` (Loki, Prometheus, Tempo) và `zenify-posthog`. Cả hai dùng `type: http` tới broker, và Claude Code lấy header đăng nhập bằng `zenify broker headers` mỗi lần kết nối, nên bạn không điền token nào. Skill `/znf:fix` dùng chúng ở Step 0 để lấy log, trace và latency khi gặp lỗi trên staging hay production.

Sau khi `zenify up --apply`, khởi động lại Claude Code để MCP gắn vào phiên.

## Sandbox của Claude Code

Chưa rõ sandbox Bash của Claude Code có chặn request đi qua WARP hay không: phần kiểm tra này đang chờ, ghi chú về sandbox sẽ được bổ sung vào trang này.

## Xem thêm

[`zenify broker token`](/reference/cli/zenify_broker_token), [`zenify broker headers`](/reference/cli/zenify_broker_headers), [`zenify redis-read`](/reference/cli/zenify_redis-read), [`zenify db-read`](/reference/cli/zenify_db-read), [`zenify e2e login`](/reference/cli/zenify_e2e_login), [Đọc dữ liệu thật](/guides/read-real-data), [Dựng host credential broker](/guides/broker-host)
