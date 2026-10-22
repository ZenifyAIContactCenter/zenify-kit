# Dựng host credential broker

Trang này dành cho maintainer: dựng `zenify-broker`, service giữ các credential của Mongo, MySQL, Redis, Grafana, PostHog và cấp cho thành viên org GitHub qua JWT. Credential đọc (`db-read`, `redis-read`) là read-only; credential ghi (tuỳ chọn, xem [Secret ghi và lumi](#secret-ghi-lumi)) chỉ đi qua `zenify data run`. Dev không cần biết các credential này; phía dev xem [Dùng credential broker](/guides/broker-client). Image `ghcr.io/zenifyaicontactcenter/zenify-broker` (linux/amd64) có sẵn `mongosh`, `mysql` client và `mcp-grafana` 2.0.2. GoReleaser build tag `:<version>` mỗi release; workflow `broker-image` build tag `:main-<sha>` và `:main` mỗi khi code broker trên `main` đổi (hoặc chạy tay), để dựng và kiểm host trước khi tag release. Binary `zenify-broker` không nằm trong brew cask hay scoop.

## Runbook 8 bước

1. Trỏ A record hostname broker (`broker.zenify.vn`) về IP public của host. Đặt nginx trên host trước broker, cấp cert bằng `certbot --nginx -d <hostname>`, và cho broker chỉ nghe trên `127.0.0.1` (xem [Reverse proxy TLS](#reverse-proxy-tls)). Lớp kiểm soát truy cập là GitHub auth của broker; khi có VPN thì giới hạn vhost theo dải VPN.
2. Tạo user read-only trên Mongo staging và prod, MySQL staging và prod, và ACL Redis read-only (`+@read -@dangerous` hoặc tương đương) cho staging và prod.
3. Tạo service-account Grafana riêng `zenify-broker` với role Viewer, có quyền Query trên datasource Loki, Prometheus, Tempo nếu Grafana bật datasource permissions. Không dùng lại token đang chia cho dev; thu hồi token cũ sau khi broker chạy ổn.
4. Tạo key PostHog với preset "MCP Server", read-only, gắn project 137053, thuộc một user bot riêng chứ không phải tài khoản cá nhân.
5. Cấp cho broker một tài khoản superadmin **staging**. Ghi `tenant_id` và `group_id` (department) của tenant `zenifytest` vào config.
6. Sinh `BROKER_JWT_KEY` và `BROKER_E2E_HMAC_KEY` ngẫu nhiên, mỗi key tối thiểu 32 byte (ví dụ `openssl rand -hex 32`).
7. Bật bắt buộc 2FA cho org GitHub.
8. Cấu hình Alloy thu stdout của container vào Loki với label `app=zenify-broker`. Đặt alert theo số row `prd` mỗi login mỗi giờ.

## Config không chứa secret

File YAML mount vào `/etc/zenify-broker/broker.yaml`. Các khoá `listen`, `lead_team`, `dev_team`, `deny_logins`, `agent_ids`, `grafana.bin`, `grafana.addr`, `grafana.args`, `mongo.main_db`, `mongo.audit_db` có mặc định; các khoá còn lại bắt buộc.

```yaml
org: ZenifyAIContactCenter
listen: ":8080"
# lead_team: leaders        # team cấp role lead (mặc định leaders)
# dev_team: zenify-team     # team cấp role dev (mặc định zenify-team)
deny_logins:                # tài khoản dùng chung, không cấp token (không phân biệt hoa thường)
  - zenifydevteam
  - zenifydeveloper
# agent_ids: [346015963]    # databaseId của bot GitHub App được cấp role agent (mặc định rỗng: không nhận token ghs_)
grafana:
  url: https://monitor.zenify.vn
  # bin: mcp-grafana
  # addr: 127.0.0.1:8000
  # args: mặc định là bộ cờ team đang dùng (--enabled-tools=..., --loki-guardrail-mode=enforce, ...)
posthog:
  upstream: https://mcp.posthog.com/mcp?features=insights,flags  # danh sách feature chỉ là ví dụ
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

Query trong `posthog.upstream` được giữ nguyên khi proxy chuyển tiếp. Nếu client gửi cùng key thì giá trị của upstream thắng; key khác của client vẫn được nối thêm. Cặp nào của client chứa dấu `;` hoặc có key không giải mã được thì bị bỏ, để client không chèn được key mà upstream giữ.

::: warning Khi nâng cấp broker
Bản broker này đổi cách chuyển tiếp một URL upstream MCP đã có sẵn query string: query của client giờ được nối thêm vào sau query của upstream (key của upstream thắng, cặp của client chứa `;` bị bỏ). Sau khi nâng cấp, kiểm tra lại từng URL upstream có query (ví dụ `posthog.upstream`) để chắc upstream vẫn nhận đúng tham số.
:::

Thay các giá trị `example.invalid` và placeholder bằng giá trị thật của staging. Broker không đọc secret từ YAML.

Role: ở `POST /token`, sau khi xác nhận member org, broker từ chối login trong `deny_logins` (403 `login_denied`), rồi hỏi GitHub membership của `lead_team` (`state=active` → `lead`), rồi `dev_team` (→ `dev`); không thuộc team nào thì 403 `not_allowed`. Trên production, dev chỉ đọc được cấu trúc và plan (explain, indexes, stats, count, schema SQL, Redis metadata); phần còn lại trả 403 `lead_only` và vẫn ghi audit. Dòng audit có field `role`. Xem [Dùng credential broker](/guides/broker-client#lead-dev).

### Role `agent` cho GitHub App {#role-agent}

Worker không người ([Chế độ không người](/guides/autonomous)) không có tài khoản người dùng. Nó gửi một installation token của GitHub App (tiền tố `ghs_`) tới `POST /token`. GitHub không cho token này gọi `/user`, nên broker hỏi GraphQL `viewer { login databaseId }` để biết bot nào đang gọi. Quy trình: `deny_logins` trước (403 `login_denied`), rồi `databaseId` của bot phải nằm trong `agent_ids` (không thì 403 `not_allowed`, "GitHub App installation is not in agent_ids"). Khớp thì broker cấp JWT role `agent`. Token `ghs_` mà GraphQL trả lỗi hoặc không có `viewer` thì nhận 401 `github_token_invalid`, không có JWT; riêng lỗi GraphQL loại `RATE_LIMITED` thì nhận 429 `rate_limited`, chờ rồi thử lại. `agent_ids` rỗng nghĩa là broker không nhận installation token nào.

Quyền của `agent`: đọc staging và production như lead (`db-read`, `redis-read`, `data run` chế độ đọc); ghi staging được; **không bao giờ** lấy được credential `--prd --write`, cũng không lấy được credential lumi-agent của production (`data run --prd --lumi`, vì đó là credential đọc-ghi của chính lumi-agent): cả hai đều nhận 403 `lead_only` ở `POST /creds`. Muốn thêm một bot, lấy `databaseId` của nó rồi thêm vào `agent_ids` và restart broker.

Mỗi request có thể mang header `X-Zenify-Task` (giá trị khớp `^[A-Za-z0-9._-]{1,64}$`; giá trị khác bị bỏ qua, request vẫn chạy). Dòng audit khi đó có thêm field `task`, để lần ngược một dòng audit về task của worker. Field `task` bỏ trống khi không có header hợp lệ.

## Secret qua env của host

Broker từ chối khởi động nếu thiếu một biến, và thông báo lỗi chỉ liệt kê tên biến, không in giá trị. `BROKER_JWT_KEY` và `BROKER_E2E_HMAC_KEY` ngắn hơn 32 byte cũng bị từ chối.

`BROKER_JWT_KEY` `BROKER_E2E_HMAC_KEY` `MONGO_URL_STG` `MONGO_URL_PRD` `MYSQL_URL_STG` `MYSQL_URL_PRD` `AUDIT_MONGO_URL_PRD` `REDIS_URL_STG` `REDIS_URL_PRD` `GRAFANA_SERVICE_ACCOUNT_TOKEN` `POSTHOG_API_KEY` `E2E_ADMIN_EMAIL` `E2E_ADMIN_PASSWORD`

## Secret ghi và lumi {#secret-ghi-lumi}

Các biến sau là tuỳ chọn: thiếu thì broker vẫn khởi động. `POST /creds` (lệnh `zenify data run`) chỉ cấp được những gì đã được đặt.

`MONGO_WRITE_URL_STG` `MONGO_WRITE_URL_PRD` `MYSQL_WRITE_URL_STG` `MYSQL_WRITE_URL_PRD` `REDIS_WRITE_URL_STG` `REDIS_WRITE_URL_PRD` `LUMI_MYSQL_URL_STG` `LUMI_MYSQL_URL_PRD` `QDRANT_URL_STG` `QDRANT_URL_PRD` `ELASTIC_URL_STG` `ELASTIC_URL_PRD`

- Ba cặp `*_WRITE_URL_*` là credential của user có quyền ghi. Thiếu chúng thì yêu cầu `--write` nhận 500 `broker_misconfigured`; yêu cầu đọc không bị ảnh hưởng. Dev chỉ lấy được credential staging; production chỉ dành cho lead.
- `LUMI_MYSQL_URL_*` và `QDRANT_URL_*` là credential của `lumi-agent`, cấp khi người dùng chạy `zenify data run --lumi`. Trước đây dev tự điền các key này vào `.zenify/.env`; nay broker cấp, manifest không còn hỏi. Broker cũng tự dùng `LUMI_MYSQL_URL_*` cho `zenify db-read --target lumi`, nên URL phải tới được từ container broker: production (`<lumi-prd-mysql-host>`) tới thẳng, staging cần [tunnel bên dưới](#lumi-stg-tunnel). Hậu tố `+pymysql` giữ nguyên, broker tự bỏ.
- `ELASTIC_URL_*` là URL Elasticsearch chỉ đọc (kèm thông tin đăng nhập nếu có). Broker chỉ cấp key `ELASTIC_URL` cho yêu cầu `read`, không bao giờ cho `--write`, và chỉ khi biến đã được đặt.
- URL ứng dụng lumi staging đặt trong YAML, không phải env: khối `lumi:` với khoá `app_url_stg`.

```yaml
lumi:
  app_url_stg: https://lumi.example.invalid
```

Chỉ thêm khối `lumi:` vào `broker.yaml` sau khi image mới (có `POST /creds`) đã chạy. Image cũ đọc config với `KnownFields` nên từ chối khoá lạ và không khởi động: trước khi quay lại một image cũ, xoá khối `lumi:` khỏi `broker.yaml`.

**URI phải dùng được từ máy dev.** Broker chuyển nguyên chuỗi kết nối cho máy dev, và script chạy trên máy dev, nên mọi URI trên (kể cả `MONGO_URL_*`, `MYSQL_URL_*`, `REDIS_URL_*` và các URI ghi) phải trỏ tới địa chỉ mà máy dev tới được (qua VPN hay hostname công khai), không phải địa chỉ chỉ host broker mới thấy như `localhost` hay IP nội bộ của docker network. Ngoại lệ có chủ đích: `LUMI_MYSQL_URL_STG` giữ nguyên `127.0.0.1:3307`; trong broker nó chạy qua container tunnel, còn trên máy dev nó chỉ dùng được khi dev tự mở SSH tunnel tới cổng local 3307 (cần cho `zenify data run --stg --lumi`).

Kiểm tra từ máy dev: chạy một lệnh chỉ in tên biến, không in giá trị.

```bash
zenify data run --stg -- sh -c 'env | grep -E "^(MONGO|MYSQL|REDIS)_URL=" | cut -d= -f1'
```

Kết quả phải liệt kê `MONGO_URL`, `MYSQL_URL`, `REDIS_URL`. Muốn chắc kết nối tới được thì chạy thêm một lệnh đọc thật, ví dụ `zenify db-read collections`. Biến ghi chỉ lộ ra khi chạy `--write` sau một dry-run, xem [Dùng credential broker](/guides/broker-client#data-run).

## Danh mục dịch vụ {#danh-muc-dich-vu}

Khối `catalog:` là tuỳ chọn. Khi có, broker phục vụ `POST /catalog` và `POST /catalog/drift` cho lệnh `zenify catalog` và `zenify catalog drift`; danh mục là một file YAML trong một repo GitHub, broker tự đọc và làm mới.

```yaml
catalog:
  repo: <owner>/<repo>
  path: <đường dẫn file trong repo>
  # ref: main               # mặc định main
```

Khi có khối này, broker bắt buộc biến env `CATALOG_GITHUB_TOKEN` và từ chối khởi động nếu thiếu. Tạo một fine-grained personal access token chỉ cho đúng một repo đó, với quyền Repository permissions, Contents: Read-only, rồi đặt vào file env của host (`chmod 600`). Khi tạo token, chọn org làm Resource owner; nếu org bắt duyệt fine-grained token thì chờ admin duyệt. Lỗi `broker 503 catalog_unavailable: catalog fetch failed: GitHub returned 404` nghĩa là token chưa thấy được repo.

Chỉ thêm khối `catalog:` vào `broker.yaml` sau khi image mới (có `/catalog`) đã chạy. Image cũ từ chối khoá lạ và không khởi động; trước khi quay lại image cũ, xoá khối `catalog:`.

Kiểm tra từ máy dev bằng `zenify catalog` (in YAML; mã thoát 5 và `no catalog configured` nghĩa là broker chưa có khối `catalog:`), hoặc gọi thẳng với JWT lấy từ `/token`:

```bash
curl -s -X POST -H "Authorization: Bearer <jwt>" -d '{}' <url>/catalog
```

`zenify obs` (tóm tắt route và trace, xem [Quan sát và an toàn](/guides/observe-and-safety#zenify-obs)) không có khối cấu hình riêng: broker phục vụ `POST /obs/route` và `POST /obs/trace` khi có khối `catalog:`, vì cần danh mục để biết tên app Loki và `service.name` Tempo của từng service. Chưa có khối `catalog:` thì `zenify obs` thoát mã 5. Image cũ chưa có `/obs` trả 404 và lệnh cũng thoát mã 5, nên nâng broker trước khi teammate dùng bản kit mới.

## Chạy bằng docker

Giữ giá trị secret trong file env của host (`chmod 600`), không đưa lên dòng lệnh.

```bash
docker run -d --name zenify-broker --restart unless-stopped \
  -p 127.0.0.1:18080:8080 \
  --env-file /etc/zenify-broker/secrets.env \
  -v /etc/zenify-broker/broker.yaml:/etc/zenify-broker/broker.yaml:ro \
  ghcr.io/zenifyaicontactcenter/zenify-broker:<tag>
```

`<tag>` là `main-<sha>` khi dựng trước release, `<version>` (ví dụ `0.39.0`) sau release. Nếu package trên ghcr ở chế độ private, chạy `docker login ghcr.io` trên host bằng một token có quyền `read:packages` trước.

Image mở cổng 8080 và chạy `zenify-broker --config /etc/zenify-broker/broker.yaml`. Nếu `mcp-grafana` thoát, broker cũng thoát (khác 0) để orchestrator khởi động lại cả hai.

### Tunnel MySQL lumi staging {#lumi-stg-tunnel}

MySQL staging của lumi (`mass_chatbot_v2`) chỉ tới được qua SSH tới `<lumi-stg-ssh-user>@<lumi-stg-ssh-host>` cổng `<lumi-stg-ssh-port>`. Đặt `LUMI_MYSQL_URL_STG` trỏ `127.0.0.1:3307` rồi chạy một container tunnel dùng chung network namespace với broker, để cổng 3307 chỉ có trong container broker và không publish ra host. Giá trị thật của `<lumi-stg-ssh-host>`, `<lumi-stg-ssh-port>`, `<lumi-stg-ssh-user>` và `<lumi-prd-mysql-host>` nằm trong ghi chú vận hành riêng của host broker, không đưa vào hướng dẫn công khai này. Không bao giờ publish cổng 3307 từ container tunnel và không bind `-L` vào địa chỉ nào khác `127.0.0.1`.

Trên host broker chạy bằng docker compose (service `zenify-broker`, thư mục `/root/zenify-broker`), thêm service:

```yaml
  lumi-stg-tunnel:
    image: alpine:3.20
    network_mode: "service:zenify-broker"
    restart: unless-stopped
    volumes:
      - /etc/zenify-broker/ssh:/ssh:ro
    command:
      - sh
      - -c
      - >-
        apk add --no-cache openssh-client &&
        exec ssh -N -o ExitOnForwardFailure=yes -o ServerAliveInterval=30 -o ServerAliveCountMax=3
        -o StrictHostKeyChecking=yes -o UserKnownHostsFile=/ssh/known_hosts -i /ssh/id_ed25519
        -p <lumi-stg-ssh-port> -L 127.0.0.1:3307:127.0.0.1:3307 <lumi-stg-ssh-user>@<lumi-stg-ssh-host>
```

Nếu broker chạy bằng `docker run --name zenify-broker` như phần trên, chạy tunnel với `--network container:zenify-broker` thay cho `network_mode`:

```bash
docker run -d --name lumi-stg-tunnel --restart unless-stopped \
  --network container:zenify-broker \
  -v /etc/zenify-broker/ssh:/ssh:ro \
  alpine:3.20 sh -c 'apk add --no-cache openssh-client && exec ssh -N -o ExitOnForwardFailure=yes -o ServerAliveInterval=30 -o ServerAliveCountMax=3 -o StrictHostKeyChecking=yes -o UserKnownHostsFile=/ssh/known_hosts -i /ssh/id_ed25519 -p <lumi-stg-ssh-port> -L 127.0.0.1:3307:127.0.0.1:3307 <lumi-stg-ssh-user>@<lumi-stg-ssh-host>'
```

Container tunnel chạy `apk add openssh-client` ở mỗi lần khởi động nên cần ra internet tới mirror Alpine; nếu không có, hãy build sẵn một image nhỏ đã cài `openssh-client`. Nên dùng image dựng sẵn với `openssh-client` ghim theo digest, vì container này giữ SSH key và dùng chung network namespace với broker.

Thư mục `/ssh` gồm `id_ed25519` (key riêng của tunnel) và `known_hosts` đã có host key của `<lumi-stg-ssh-host>:<lumi-stg-ssh-port>`.

Trên host SSH của lumi staging (user `<lumi-stg-ssh-user>`), thêm vào `authorized_keys` một dòng chỉ cho phép forward tới đúng cổng MySQL, không cho chạy lệnh:

```text
restrict,port-forwarding,permitopen="127.0.0.1:3307",command="/bin/false" ssh-ed25519 AAAA… lumi-stg-tunnel
```

Tunnel dùng network của container broker lúc nó được tạo. Recreate broker bằng `docker compose up -d` thì compose recreate cả tunnel; `docker restart zenify-broker` đơn lẻ thì phải restart thêm `lumi-stg-tunnel`.

Kiểm tra từ máy dev:

```bash
zenify db-read --target lumi sql 'SELECT DATABASE()'
```

Kết quả phải là `mass_chatbot_v2`.

## Reverse proxy TLS

Kit chỉ nhận `broker.url` dạng `https://`, còn broker phục vụ HTTP thường, nên một reverse proxy trên host lo TLS. Broker giới hạn `/token` theo IP lấy từ header `X-Real-IP`, vì vậy proxy **phải ghi đè** header này bằng địa chỉ thật của client, và broker không được mở cổng nào ra ngoài `127.0.0.1` (nếu không, client tự gửi `X-Real-IP` giả để né rate limit).

```nginx
server {
    listen 80;
    server_name broker.zenify.vn;
    location / {
        proxy_pass http://127.0.0.1:18080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 300s;
    }
}
```

Sau đó `nginx -t && systemctl reload nginx` rồi `certbot --nginx -d broker.zenify.vn`; certbot thêm khối `listen 443 ssl` và redirect từ 80.

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
