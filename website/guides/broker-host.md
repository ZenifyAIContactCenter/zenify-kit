# Dựng host credential broker

Trang này dành cho maintainer: dựng `zenify-broker`, service giữ các credential read-only (Mongo, MySQL, Redis, Grafana, PostHog) và cấp cho thành viên org GitHub qua JWT. Dev không cần biết các credential này; phía dev xem [Dùng credential broker](/guides/broker-client). Image `ghcr.io/zenifyaicontactcenter/zenify-broker` (linux/amd64) có sẵn `mongosh`, `mysql` client và `mcp-grafana` 2.0.2. GoReleaser build tag `:<version>` mỗi release; workflow `broker-image` build tag `:main-<sha>` và `:main` mỗi khi code broker trên `main` đổi (hoặc chạy tay), để dựng và kiểm host trước khi tag release. Binary `zenify-broker` không nằm trong brew cask hay scoop.

## Runbook 8 bước

1. Tạo Cloudflare Tunnel từ host broker, cùng một Access application cho hostname broker. Policy: IdP GitHub, org `ZenifyAIContactCenter`. Bật WARP enrollment với cùng policy.
2. Tạo user read-only trên Mongo staging và prod, MySQL staging và prod, và ACL Redis read-only (`+@read -@dangerous` hoặc tương đương) cho staging và prod.
3. Tạo service-account Grafana riêng `zenify-broker` với role Viewer, có quyền Query trên datasource Loki, Prometheus, Tempo nếu Grafana bật datasource permissions. Không dùng lại token đang chia cho dev; thu hồi token cũ sau khi broker chạy ổn.
4. Tạo key PostHog với preset "MCP Server", read-only, gắn project 137053, thuộc một user bot riêng chứ không phải tài khoản cá nhân.
5. Cấp cho broker một tài khoản superadmin **staging**. Ghi `tenant_id` và `group_id` (department) của tenant `zenifytest` vào config.
6. Sinh `BROKER_JWT_KEY` và `BROKER_E2E_HMAC_KEY` ngẫu nhiên, mỗi key tối thiểu 32 byte (ví dụ `openssl rand -hex 32`).
7. Bật bắt buộc 2FA cho org GitHub.
8. Cấu hình Alloy thu stdout của container vào Loki với label `app=zenify-broker`. Đặt alert theo số row `prd` mỗi login mỗi giờ.

## Config không chứa secret

File YAML mount vào `/etc/zenify-broker/broker.yaml`. Các khoá `listen`, `grafana.bin`, `grafana.addr`, `grafana.args`, `mongo.main_db`, `mongo.audit_db` có mặc định; các khoá còn lại bắt buộc.

```yaml
org: ZenifyAIContactCenter
listen: ":8080"
grafana:
  url: https://monitor.zenify.vn
  # bin: mcp-grafana
  # addr: 127.0.0.1:8000
  # args: mặc định là bộ cờ team đang dùng (--enabled-tools=..., --loki-guardrail-mode=enforce, ...)
posthog:
  upstream: https://mcp.posthog.com/mcp
  project_id: "137053"
e2e:
  hub_url: https://hub.example.invalid
  be_url: https://be.example.invalid
  domain: zenifytest
  tenant_id: "<tenant_id của zenifytest>"
  group_id: "<group_id (department)>"
  email_domain: e2e.example.invalid
mongo:
  main_db: 3csoft
  audit_db: audit_logs
```

Thay các giá trị `example.invalid` và placeholder bằng giá trị thật của staging. Broker không đọc secret từ YAML.

## Secret qua env của host

Broker từ chối khởi động nếu thiếu một biến, và thông báo lỗi chỉ liệt kê tên biến, không in giá trị. `BROKER_JWT_KEY` và `BROKER_E2E_HMAC_KEY` ngắn hơn 32 byte cũng bị từ chối.

`BROKER_JWT_KEY` `BROKER_E2E_HMAC_KEY` `MONGO_URL_STG` `MONGO_URL_PRD` `MYSQL_URL_STG` `MYSQL_URL_PRD` `AUDIT_MONGO_URL_PRD` `REDIS_URL_STG` `REDIS_URL_PRD` `GRAFANA_SERVICE_ACCOUNT_TOKEN` `POSTHOG_API_KEY` `E2E_ADMIN_EMAIL` `E2E_ADMIN_PASSWORD`

## Chạy bằng docker

Giữ giá trị secret trong file env của host (`chmod 600`), không đưa lên dòng lệnh.

```bash
docker run -d --name zenify-broker --restart unless-stopped \
  -p 127.0.0.1:8080:8080 \
  --env-file /etc/zenify-broker/secrets.env \
  -v /etc/zenify-broker/broker.yaml:/etc/zenify-broker/broker.yaml:ro \
  ghcr.io/zenifyaicontactcenter/zenify-broker:<tag>
```

`<tag>` là `main-<sha>` khi dựng trước release, `<version>` (ví dụ `0.39.0`) sau release. Nếu package trên ghcr ở chế độ private, chạy `docker login ghcr.io` trên host bằng một token có quyền `read:packages` trước.

Image mở cổng 8080 và chạy `zenify-broker --config /etc/zenify-broker/broker.yaml`. Nếu `mcp-grafana` thoát, broker cũng thoát (khác 0) để orchestrator khởi động lại cả hai.

## Kiểm tra trước khi tag release

Manifest trên `main` đã khai `broker:`, nên bản release kế tiếp chuyển mọi dev sang broker. Làm bước này trên host chạy image `:main-<sha>` trước khi tag (FR-13.4):

1. `curl <url>/healthz` trả `ok`.
2. Gọi thẳng broker một request thật (bản `zenify` đã cài vẫn là bản release cũ, chạy local, nên không dùng `zenify db-read` để kiểm tra). Lấy JWT bằng GitHub token của bạn, rồi gọi `/query`:

   ```bash
   curl -s <url>/token -H 'content-type: application/json' -d "{\"github_token\":\"$(gh auth token)\"}"
   curl -s -H "Authorization: Bearer <jwt>" <url>/query -d '{"env":"stg","engine":"mongo","op":"collections"}'
   ```

   Lệnh đầu trả `{"token":"<jwt>",...}`; lệnh sau trả danh sách collection của staging.
3. Mở Grafana Explore, datasource Loki, truy vấn `{app="zenify-broker"}` và xác nhận có dòng audit của request vừa chạy (login, endpoint `/query`, `status` 200). Chưa thấy dòng audit thì chưa tag — kiểm tra lại bước 8 của runbook.
