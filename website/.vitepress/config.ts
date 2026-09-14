import { withMermaid } from 'vitepress-plugin-mermaid'

// Sidebar chỉ trỏ trang TỒN TẠI (spec FR-3.2). Task 7/8/9 thêm item vào các mảng dưới. Thứ tự nhóm cố định: Bắt đầu → Quy trình → Khái niệm → Tham chiếu.
// withMermaid bọc config để render fence ```mermaid (FR-3.1/FR-3.5)
export default withMermaid({
  lang: 'vi-VN',
  title: 'zenify-kit',
  description: 'Tài liệu nội bộ bộ công cụ workspace của team zenify',
  cleanUrls: true,
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
      { text: 'Bắt đầu', items: [] },
      { text: 'Quy trình', items: [] },
      { text: 'Khái niệm cốt lõi', items: [] },
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
