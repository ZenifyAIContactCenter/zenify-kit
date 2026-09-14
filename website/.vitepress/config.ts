import { withMermaid } from 'vitepress-plugin-mermaid'

// Sidebar chỉ trỏ trang TỒN TẠI (spec FR-3.2). Task 7/8/9 thêm item vào các mảng dưới. Thứ tự nhóm cố định: Bắt đầu → Quy trình → Khái niệm → Tham chiếu.
// withMermaid bọc config để render fence ```mermaid (FR-3.1/FR-3.5)
export default withMermaid({
  lang: 'vi-VN',
  title: 'zenify-kit',
  description: 'Tài liệu nội bộ bộ công cụ workspace của team zenify',
  cleanUrls: true,
  lastUpdated: true,
  ignoreDeadLinks: false,
  srcExclude: ['node_modules/**'],
  themeConfig: {
    search: { provider: 'local' },
    outline: { label: 'Trong trang này', level: [2, 3] },
    docFooter: { prev: 'Trang trước', next: 'Trang sau' },
    lastUpdated: { text: 'Cập nhật' },
    nav: [
      { text: 'Bắt đầu', link: '/getting-started/install' },
      { text: 'Tham chiếu', link: '/reference/cli/' },
    ],
    sidebar: [
      { text: 'Bắt đầu', items: [
        { text: 'zenify-kit là gì', link: '/' },
        { text: 'Cài đặt', link: '/getting-started/install' },
        { text: 'Bắt đầu nhanh', link: '/getting-started/quickstart' },
        { text: 'Nâng cấp', link: '/getting-started/upgrade' } ] },
      { text: 'Quy trình', items: [
        { text: 'Chọn quy trình', link: '/workflows/' },
        { text: 'cook — xây tính năng', link: '/workflows/cook' },
        { text: 'fix — sửa lỗi chưa rõ nguyên nhân', link: '/workflows/fix' },
        { text: 'hotfix — lỗi đang chạy trên production', link: '/workflows/hotfix' },
        { text: 'ship — cổng cuối, mở PR', link: '/workflows/ship' } ] },
      { text: 'Khái niệm cốt lõi', items: [
        { text: 'Ba lớp: binary, plugin, knowledge store', link: '/concepts/three-layers' },
        { text: 'Worktree theo slug', link: '/concepts/worktree-per-slug' },
        { text: 'Base ref, hotfix base, port block', link: '/concepts/base-ref-and-ports' },
        { text: 'Knowledge store và view docs/', link: '/concepts/knowledge-store' },
        { text: 'Namespace znf:', link: '/concepts/znf-namespace' },
        { text: 'Gate fail-open', link: '/concepts/gate-fail-open' } ] },
      {
        text: 'Tham chiếu (sinh tự động)',
        items: [
          { text: 'Lệnh CLI', link: '/reference/cli/' },
          { text: 'Skill', link: '/reference/skills/' },
          { text: 'Agent', link: '/reference/agents/' },
          { text: 'Hook', link: '/reference/hooks' },
        ],
      },
    ],
  },
})
