---
title: /znf:hotfix — lỗi đang chạy trên production
---

# /znf:hotfix

## Dùng khi / Không dùng khi

| Dùng khi | Không dùng khi |
|---|---|
| Lỗi đang ảnh hưởng production và bạn xác nhận độ khẩn | Lỗi trên staging/dev — [`/znf:fix`](/workflows/fix) |

## Cách gọi

```text
/znf:hotfix <mô tả ngắn>
```

`/znf:hotfix` **chỉ bạn gõ được** — frontmatter `disable-model-invocation: true`, agent không tự chạy. Nếu một lỗi trông có vẻ khẩn cấp, agent chỉ được **đề xuất** chạy hotfix, quyết định độ khẩn là của bạn.

## Viết yêu cầu cho tốt

| Trường | Ví dụ |
|---|---|
| Repo | Repo đang chạy bản lỗi |
| Release đang chạy | Bản đang deploy production, chưa phải nhánh tích hợp thường ngày |
| Ảnh hưởng | Ai/luồng nào đang bị chặn |
| Bằng chứng | Log thật, ticket, `docker logs <container>` |

## Diễn biến một lần chạy

```mermaid
flowchart TD
  A[1: chốt repo ảnh hưởng] --> B[2: fetch, resolve base ref release]
  B --> C{Bạn xác nhận<br/>base ref đúng chưa?}
  C -- xác nhận --> D[3: chẩn đoán — bước 0-2 của fix]
  D --> E{Bạn chọn:<br/>revert / disable / fix forward?}
  E -- revert hoặc disable --> F[Dừng: không worktree,<br/>không scout, không sửa]
  E -- fix forward --> G[5: worktree --type hotfix --base release]
  G --> H[6: scout trên ref đã resolve]
  H --> I[7: sửa nhỏ nhất]
  I --> J[8: verify]
  J --> K[9: gate rồi ship]
  F --> K
  K --> L[10: nhắc bước tay:<br/>PR vào release, đồng bộ ngược]
```

| Bước | Kit làm gì | Bạn thấy / làm gì |
|---|---|---|
| 1 | Chốt repo bị ảnh hưởng | Xác nhận đúng repo |
| 2 | `fetch` trước rồi resolve base ref hotfix (release mới nhất qua `zenify hotfix baseref`) | **Dừng hỏi: bạn xác nhận đây đúng là bản đang chạy production** |
| 3 | Chẩn đoán bằng bước 0-2 của `/znf:fix` (log thật → hypothesis → kiểm chứng) | Đọc nguyên nhân đã xác nhận |
| 4 | Trình bày ba lựa chọn kèm khuyến nghị | **Dừng hỏi: bạn chọn revert / disable / fix forward** — revert/disable thì dừng ở đây, không tạo worktree |
| 5 | Tạo worktree `--type hotfix --base <release đã resolve>` (chỉ khi fix forward) | Không cần làm gì |
| 6 | `/znf:scout` trên đúng ref đã resolve, không phải base thường ngày | Đọc báo cáo scout |
| 7 | Sửa nhỏ nhất | Không cần làm gì |
| 8 | Verify trên code path thật | Đọc kết quả |
| 9 | `/znf:gate` rồi `/znf:ship`, luôn luôn, kể cả đang gấp | Đọc board ship, nhận PR |
| 10 | Nhắc bước tay còn lại | **Bạn tự tay**: mở PR vào release, merge (= deploy), rồi đồng bộ fix ngược về base feature (merge/cherry-pick) |

## Kết quả nhận được

| Mục | Nội dung |
|---|---|
| Base ref xác nhận | Release đang chạy, ví dụ `origin/release<N>` |
| Nguyên nhân xác nhận | Kèm bằng chứng thật |
| Response đã chọn | revert / disable / fix forward, kèm lý do |
| Branch (nếu fix forward) | `<user>/hotfix/<slug>` |
| PR | Mở vào base ref release, chưa merge |

## Việc chỉ bạn quyết định

- Đây có phải hotfix thật không — độ khẩn là quyết định của bạn, không phải của agent.
- Base ref đúng là bản đang chạy.
- Chọn revert / disable / fix forward.
- Merge PR, và đồng bộ fix ngược về base feature.

## Ví dụ

**Minh hoạ** (repo kit chưa có hotfix thật, dùng số release giữ chỗ): production đang chạy `release<N>` gặp lỗi xác nhận qua log thật. Bạn xác nhận base ref là `origin/release<N>`, chọn fix forward vì nguyên nhân đã rõ và không cần quyết định thiết kế. Kit tạo worktree `--type hotfix --base origin/release<N>`, scout trên đúng ref đó, sửa, verify, gate rồi ship; PR mở vào `release<N>`. Sau khi bạn merge, bạn tự cherry-pick fix về base feature để bản release kế tiếp không mất nó.

## Tránh / Nên làm

| Tránh | Nên làm |
|---|---|
| Resolve release rồi mới fetch | Luôn `fetch` trước, resolve base ref sau — nếu không, resolve có thể ra một ref cũ hơn bản đang chạy |
| Sửa tiến dưới áp lực khi còn phân vân | Chọn revert khi sửa tiến cần một quyết định thiết kế, không phải lúc đang gấp |
| Quên đồng bộ ngược về base feature | Bước 10 luôn nhắc — release sau vẫn cần fix này |

## Nguồn

- `internal/plugin/assets/znf/skills/hotfix/SKILL.md @ b296ca1`
- Xem thêm: [`/reference/skills/hotfix`](/reference/skills/hotfix), [Base ref, hotfix base, port block](/concepts/base-ref-and-ports), [Chọn quy trình](/workflows/)
- Ground trên binary build từ commit b296ca1 của nhánh này (2026-09-14), chưa phát hành.
