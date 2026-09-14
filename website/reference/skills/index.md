---
title: Skill
---

# Skill

Danh sách skill của ZenifyKit, nhóm theo znf và coding.

Skill của kit dùng tiền tố `/znf:`. Các skill coding (quy ước viết code theo stack) dùng được sau khi chạy `zenify skills install`. Skill được dùng nhiều nhất là cook, fix, và hotfix — xem [Chọn workflow](/workflows/) nếu chưa biết bắt đầu từ đâu.

## Danh sách

| Tên | Mô tả |
|---|---|
| [/express-service-patterns](./express-service-patterns) | Quy ước viết backend Express legacy — controller/service, response envelope, xử lý lỗi bất đồng bộ. |
| [/mongo-data-safety](./mongo-data-safety) | Đọc/ghi MongoDB an toàn trong một codebase đa tenant không có schema cứng. |
| [/mongoose-modeling](./mongoose-modeling) | Thiết kế schema Mongoose an toàn — index, lean/populate, discriminator, thứ tự thêm field mới. |
| [/nestjs-patterns](./nestjs-patterns) | Quy ước viết backend NestJS — module, DTO, guard, truy cập model qua DI, envelope lỗi thống nhất. |
| [/react-patterns](./react-patterns) | Quy ước viết frontend React (Vite/TanStack) — state, bảng phân trang, form, axios interceptor, i18n. |
| [/service-integration](./service-integration) | An toàn khi đổi một hợp đồng liên dịch vụ — payload pub/sub, job BullMQ, hoặc shape HTTP. |
| [/sql-data-safety](./sql-data-safety) | Đọc/ghi SQL an toàn qua connection pool và SQL viết tay — bộ lọc tenant, tham số hoá, transaction. |
| [/znf:analyze](./analyze) | Kiểm tra cặp spec và plan trước khi code: độ phủ FR sang task, marker chưa làm rõ, cấu trúc Brief và bốn tiêu chí chất lượng. |
| [/znf:brainstorming](./brainstorming) | Biến ý tưởng thành thiết kế đã được bạn duyệt qua hội thoại, phân loại việc thành spike, bounded hoặc architectural. |
| [/znf:contract-sweep](./contract-sweep) | Quét một contract chung qua nhiều repo, mỗi repo một agent, phán BREAKING, RISKY hoặc SAFE cho từng chỗ dùng. |
| [/znf:cook](./cook) | Pipeline xây tính năng trọn vẹn: ground, brainstorm ra spec, scout, plan, implement bằng subagent, rồi ship. |
| [/znf:discipline](./discipline) | Bộ quy tắc làm việc thường trực của kit: định tuyến theo blast radius, không bịa, verify trước khi báo xong, worktree cho mọi thay đổi, git safety. |
| [/znf:e2e](./e2e) | Quyết định và viết journey E2E Playwright chạy thật từ UI tới BE, khẳng định kết quả domain bằng API re-fetch, qua được `zenify e2e lint`. |
| [/znf:explain-plan](./explain-plan) | DB-perf gate hai tầng: quét tĩnh diff và explain plan từng query, phân loại phát hiện BLOCKING hoặc ADVISORY. |
| [/znf:finishing-a-development-branch](./finishing-a-development-branch) | Kết thúc một branch sau khi test xanh: chọn merge local, push và mở PR, hoặc giữ nguyên; dọn worktree khi cần. |
| [/znf:fix](./fix) | Chẩn đoán trước, sửa sau. |
| [/znf:gate](./gate) | Cổng kiểm hợp đồng chung giữa các repo trong một polyrepo, chạy ngay sau khi sửa thứ gì đó dùng chung. |
| [/znf:ground](./ground) | Xác minh hình dạng và giá trị thật trước khi viết code chạm vào dữ liệu, API, hoặc code có sẵn. |
| [/znf:hotfix](./hotfix) | Xử lý một lỗi đang xảy ra trên production trong một polyrepo, từ chẩn đoán tới PR, không bao giờ tự chạy. |
| [/znf:onboard-project](./onboard-project) | Dựng hoặc vẽ lại bản đồ hệ thống của một project trước khi làm việc tính năng trên đó. |
| [/znf:prune-memory](./prune-memory) | Rà soát và dọn bộ nhớ tự động trong một lượt, gộp bản trùng và xoá ghi chú đã cũ. |
| [/znf:review-changes](./review-changes) | Review đối kháng nhiều khía cạnh cho diff lớn, xác minh chéo từng phát hiện nghiêm trọng. |
| [/znf:review](./review) | Engine review hợp nhất của kit, tự chọn mức độ soi theo diff rồi trả về danh sách phát hiện. |
| [/znf:run](./run) | Chạy app thật và lấy output thật từ đường code thật, để một thay đổi được xác minh chứ không chỉ khẳng định suông. |
| [/znf:scout](./scout) | Vẽ bản đồ những gì phụ thuộc vào một thứ, trước khi bạn thay đổi nó. |
| [/znf:ship](./ship) | Cổng trước khi ship — lint, build, gate hợp đồng, xác minh hành vi, và review độc lập, rồi commit, push và mở PR. |
| [/znf:standards](./standards) | Kiểm sau khi implement xong một plan, xem mỗi yêu cầu có test thật hay không. |
| [/znf:subagent-driven-development](./subagent-driven-development) | Thực thi một plan bằng cách dispatch một subagent implementer riêng cho mỗi task, kèm review sau mỗi task. |
| [/znf:sweep](./sweep) | Dọn dẹp một task đã xong — dừng dev server, đóng workspace, xoá worktree và branch, trên mọi repo task đó chạm tới. |
| [/znf:understand-codebase](./understand-codebase) | Đọc song song từng phần của một codebase lạ để dựng bản đồ cấu trúc ban đầu. |
| [/znf:using-zenify-kit](./using-zenify-kit) | Cách ZenifyKit chọn skill nào cho việc gì, dựa trên thứ bạn chưa biết chứ không phải quy mô việc. |
| [/znf:writing-plans](./writing-plans) | Viết plan thực thi chi tiết từ một spec, trước khi chạm vào code. |
