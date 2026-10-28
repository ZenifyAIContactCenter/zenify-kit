---
title: Release và báo cáo
---

# Release và báo cáo

`zenify release-note` ghi metadata rủi ro cho từng thay đổi khi ship. `zenify release-report` gom các ghi chú đó thành một báo cáo rủi ro cho một release tag, đọc thẳng từ git.

## Khi nào dùng

Bạn hiếm khi gõ `zenify release-note` tay. `/znf:ship` gọi lệnh này ở bước cuối, trước khi push branch, lấy giá trị từ ba tag rủi ro trong Brief của spec. Bạn chỉ chạy tay khi ship không có spec để lấy giá trị, hoặc khi đi đường không dùng workflow.

Chạy `zenify release-report` sau khi đánh tag release `vYYMMDD.n` trên `main` và trước khi deploy tag đó. Bản tuần là một tag, mỗi hotfix là một PR vào `main` rồi thêm một tag nữa, ví dụ `v261028.0` rồi `v261028.2`.

## Các bước

```mermaid
flowchart TD
  A["ship note lại rủi ro"] --> B["Commit chore(release: note ...)"]
  B --> C["zenify release-report tag"]
  C --> D["Đọc tag .md trong knowledge store"]
  class A,B,C action
  class D user
```

*Từ một lần ship đến báo cáo release*

## Lệnh

### `zenify release-note`

| Cờ | Ý nghĩa |
|---|---|
| `--slug` | Slug của thay đổi. Bắt buộc. |
| `--note` | Mô tả một dòng. |
| `--blast` | Phạm vi ảnh hưởng. Mặc định `unknown`. |
| `--db` | Thay đổi DB. Mặc định `N/A`. |
| `--rollback` | Cách rollback. Mặc định `revert PR`. |
| `--spec` | Đường dẫn spec, tùy chọn. |
| `--dir` | Thư mục repo. Mặc định thư mục hiện tại. |

```bash
zenify release-note --slug kit-docs-site --note "Site tài liệu VitePress" \
  --blast "zenify-kit" --db "N/A" --rollback "revert PR"
```

Thiếu `--slug` thì lệnh bỏ qua và không chặn ship. Commit lỗi cũng không chặn ship.

### `zenify release-report`

| Cờ | Ý nghĩa |
|---|---|
| `--out-dir` | Thư mục ghi báo cáo. Mặc định thư mục releases của knowledge store. |
| `--workspace` | Thư mục workspace. Mặc định thư mục hiện tại. |
| `--no-fetch` | Không fetch, dùng ref local. |
| `--verbose` | Hiện cả commit chore và chi tiết từng commit. |

```bash
zenify release-report v261104.0
```

Mỗi repo được tính trên khoảng từ tag liền trước tới tag bạn truyền, kể cả tag hotfix: nếu giữa tuần có `v261028.2` thì báo cáo `v261104.0` bắt đầu từ `v261028.2`, nên một tuần có hotfix ra nhiều file báo cáo. Không truyền tag, mỗi repo dùng tag mới nhất của nó và file mang tên tag mới nhất trong số đó. Repo không có tag, hoặc không có tag bạn truyền, có dòng "chưa có tag" trong báo cáo thay vì bị bỏ qua. Không repo nào có tag thì lệnh báo ra stderr, không ghi file và kết thúc bình thường.

## Kết quả

| Artifact | Nội dung |
|---|---|
| Commit `chore(release): note <slug>` | Trailer gồm slug, mô tả, blast radius, DB, cách rollback, đường dẫn spec nếu có |
| `<tag>.md` | Báo cáo cho một tag, ví dụ `v261104.0.md`, ghi vào thư mục releases của knowledge store |

Mỗi dòng trong báo cáo lấy metadata từ commit ghi chú của `zenify release-note`. Thay đổi có trong release nhưng chưa có trên `staging` được đánh dấu là regression, để bạn biết một hotfix chưa sync ngược.

## Cắt release cho zenify-kit

Release của kit là một tag `v*` đẩy lên `main`: workflow `release` chạy GoReleaser, dựng binary cho 6 target, cập nhật Homebrew cask và Scoop manifest ngay trong repo. Tag đẩy lên là đã release, không rút lại được, nên mọi việc kiểm phải xong trước khi tag.

```mermaid
flowchart TD
  A["git log last-tag..origin/main"] --> B{"feat/fix nào chưa có trang trên site?"}
  B -->|có| C["PR cập nhật website/ → merge"]
  B -->|không| D["docs gen --check + docs:build xanh trên main"]
  C --> D
  D --> E["git tag vX.Y.Z origin/main; git push origin vX.Y.Z"]
  E --> F["gh run watch → gh release view: 7 asset"]
  F --> G["brew upgrade --cask zenify && zenify up --apply"]
  class A,C,D,F action
  class B,E,G user
```

*Từ main tới binary trên máy teammate*

1. **Liệt kê thay đổi:** `git log --oneline <tag cuối>..origin/main`. Với mỗi commit `feat`/`fix`, nêu trang trên site mô tả nó. Trang `reference/**` sinh tự động và đã được `docs gen --check` chặn trong CI; các trang `concepts/`, `workflows/`, `guides/`, `getting-started/` viết tay và **không có gate cơ học**, nên đây là bước người phải làm. Skill hoặc agent mới cần đủ ba nơi: fragment trong `internal/docsgen/catalog/{skills,agents}/<tên>.md`, trang `reference/**` sinh lại, và ít nhất một liên kết từ trang viết tay (bảng skill ở [Chọn workflow](/workflows/) hoặc mục "Xem thêm" của trang concept).
2. **Đóng khoảng trống trước khi tag:** thiếu trang thì mở PR docs, merge, rồi mới tiếp. Trong PR đó viết mục `## vX.Y.Z` cho bản sắp cắt vào [Changelog](/changelog): gạch đầu dòng tiếng Việt cho người dùng kit, không dán commit. Rule `kit-release-docs` trong knowledge store nhắc agent điều này mỗi khi sửa kit; hành vi bị bỏ (một bước, một cờ, một gate) phải xoá câu tương ứng trên site trong cùng PR.
3. **Kiểm `main`:** `zenify docs gen --check` khớp, `npm run docs:build` build xong, CI `main` xanh.
4. **Đánh số:** có `feat` → tăng minor (`v0.23.0` → `v0.24.0`); chỉ `fix` → tăng patch.
5. **Tag và đẩy:** `git tag vX.Y.Z origin/main && git push origin vX.Y.Z`. Theo dõi `gh run list --workflow release`; `gh release view vX.Y.Z` phải có `checksums.txt` cùng 6 archive (3 OS × 2 arch), và commit cask/manifest xuất hiện trên `main`. Workflow release từ chối tag không có mục `## vX.Y.Z` trong `website/changelog.md`, và dùng mục đó làm nội dung GitHub Release.
6. **Phân phối:** teammate chạy `brew upgrade --cask zenify && zenify up --apply` (hoặc `zenify update`). Skill mới tới máy qua `zenify skills sync`, hook mới qua `zenify up`.

::: warning Không tag khi còn khoảng trống docs
Tag là quyết định release của người, không của agent. Agent chuẩn bị PR docs và danh sách khoảng trống; lệnh `git push origin vX.Y.Z` do bạn chạy hoặc bảo agent chạy rõ ràng.
:::

## Lưu ý

`zenify release-report` chỉ đọc git, không tự sửa gì và không chặn deploy. Đây là artifact để bạn tự quyết định release có an toàn hay không.

## Xem thêm

[Changelog](/changelog), [`/reference/cli/zenify_release-note`](/reference/cli/zenify_release-note), [`/reference/cli/zenify_release-report`](/reference/cli/zenify_release-report), [ship: verify và mở PR](/workflows/ship)

<!-- Nguồn (cho người bảo trì, không hiển thị):
- internal/docsgen/catalog/cli/zenify_release-note.md, zenify_release-report.md
-->
