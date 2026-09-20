import { withMermaid } from 'vitepress-plugin-mermaid'

// Sidebar chỉ trỏ trang TỒN TẠI (spec FR-3.2). Thứ tự nhóm: Bắt đầu → Khái niệm → Workflow → Sử dụng → Tham chiếu.
// IA mượn từ Claude Code docs (getting-started → core concepts → phần doing → reference): Khái niệm đứng sớm để định hướng người mới, nhưng gập sẵn nên không đẩy Workflow xuống sâu.
// withMermaid bọc config để render fence ```mermaid (FR-3.1/FR-3.5)
export default withMermaid({
  lang: 'vi-VN',
  appearance: 'dark', // mặc định tối, người xem vẫn bật sáng được
  head: [
    ['link', { rel: 'preconnect', href: 'https://fonts.googleapis.com' }],
    ['link', { rel: 'stylesheet', href: 'https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=Source+Serif+4:opsz,wght@8..60,400;8..60,500&family=JetBrains+Mono:wght@400;500&display=swap' }],
  ],
  title: 'ZenifyKit',
  description: 'Tài liệu nội bộ bộ công cụ workspace của team zenify',
  cleanUrls: true,
  lastUpdated: true,
  ignoreDeadLinks: false,
  srcExclude: ['node_modules/**'],
  // Dev-only: Vite phải pre-bundle mermaid, nếu không `fastdom` (CJS) import default lỗi → trang trắng. Build không bị.
  vite: { optimizeDeps: { include: ['mermaid'] } },
  // Plugin ép theme "dark" khi site tối, nên override thẳng các biến theme dark đặt cứng (mainBkg, nodeBorder...).
  mermaid: {
    theme: 'base',
    themeVariables: {
      darkMode: true,
      fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
      fontSize: '16px',
      background: '#24231f',
      mainBkg: '#33322d',
      nodeBkg: '#33322d',
      nodeBorder: '#57554e',
      nodeTextColor: '#ebe8e1',
      primaryColor: '#33322d',
      primaryTextColor: '#ebe8e1',
      primaryBorderColor: '#57554e',
      secondaryColor: '#33322d',
      tertiaryColor: '#24231f',
      textColor: '#ebe8e1',
      titleColor: '#ebe8e1',
      lineColor: '#a39d90',
      arrowheadColor: '#a39d90',
      edgeLabelBackground: '#24231f',
      clusterBkg: '#1f1e1a',
      clusterBorder: '#57554e',
    },
    // Ba lớp màu dùng chung: `class X action` (xanh, bước kit làm), `user` (đồng, việc của người), `stop` (đỏ nhạt, cổng chặn).
    themeCSS: [
      '.node.action rect{fill:#3b5a3f;stroke:#6f9a72}',
      '.node.user rect{fill:#4a3526;stroke:#d97757;stroke-dasharray:4 3}',
      '.node.stop rect{fill:#4d2f2d;stroke:#c2675f}',
      '.edgeLabel{color:#b9b4a9}',
    ].join(''),
    flowchart: { curve: 'basis', nodeSpacing: 40, rankSpacing: 48, padding: 16, htmlLabels: true, useMaxWidth: false },
  },
  themeConfig: {
    search: { provider: 'local' },
    outline: { label: 'Trong trang này', level: [2, 3] },
    docFooter: { prev: 'Trang trước', next: 'Trang sau' },
    lastUpdated: { text: 'Cập nhật' },
    nav: [
      { text: 'Bắt đầu', link: '/getting-started/install' },
      { text: 'Khái niệm', link: '/concepts/three-layers' },
      { text: 'Workflow', link: '/workflows/' },
      { text: 'Sử dụng', link: '/guides/onboard-workspace' },
      { text: 'Tham chiếu', link: '/reference/cli/' },
    ],
    sidebar: [
      // Bắt đầu: KHÔNG gập được (không có key collapsed) — luôn mở. Chỉ giữ thứ quan trọng nhất cho người mới.
      // "Nâng cấp" (ít quan trọng) đẩy xuống nhóm Sử dụng. Không kéo "Ba lớp" lên đây: trang "ZenifyKit là gì" đã brief qua ba lớp rồi.
      { text: 'Bắt đầu', items: [
        { text: 'ZenifyKit là gì', link: '/' },
        { text: 'Cài đặt', link: '/getting-started/install' },
        { text: 'Bắt đầu nhanh', link: '/getting-started/quickstart' } ] },
      // 4 nhóm dưới đều gập được, mặc định MỞ (collapsed: false = có nút gập, khởi tạo mở).
      { text: 'Khái niệm', collapsed: false, items: [
        { text: 'Ba lớp: binary, plugin, knowledge store', link: '/concepts/three-layers' },
        { text: 'Namespace znf:', link: '/concepts/znf-namespace' },
        { text: 'Worktree theo slug', link: '/concepts/worktree-per-slug' },
        { text: 'Base ref, hotfix base, port block', link: '/concepts/base-ref-and-ports' },
        { text: 'Knowledge store và docs/', link: '/concepts/knowledge-store' },
        { text: 'Gate fail-open', link: '/concepts/gate-fail-open' },
        { text: 'Chọn model: session, skill, subagent', link: '/concepts/model-routing' },
        { text: 'Ngân sách context: read-guard, meter, statusline', link: '/concepts/context-budget' },
        { text: 'Research có kiểm chứng', link: '/concepts/research' } ] },
      // Workflow: thứ dùng hàng ngày, gọi bằng tên. Chỉ gồm 4 skill + trang chọn.
      { text: 'Workflow', collapsed: false, items: [
        { text: 'Chọn workflow', link: '/workflows/' },
        { text: 'cook: xây tính năng', link: '/workflows/cook' },
        { text: 'fix: sửa lỗi chưa rõ nguyên nhân', link: '/workflows/fix' },
        { text: 'hotfix: sửa lỗi trên production', link: '/workflows/hotfix' },
        { text: 'ship: verify và mở PR', link: '/workflows/ship' } ] },
      // Sử dụng: how-to theo mảng năng lực của kit + bảo trì (nâng cấp); "Kiểm thử UI" là how-to (không phải skill) nên nằm đây, không nằm cùng 4 skill.
      { text: 'Sử dụng', collapsed: false, items: [
        { text: 'Onboard workspace và repo', link: '/guides/onboard-workspace' },
        { text: 'Đọc dữ liệu thật', link: '/guides/read-real-data' },
        { text: 'Review và gate', link: '/guides/review-and-gates' },
        { text: 'Spec, plan và kiểm tra', link: '/guides/spec-and-plan' },
        { text: 'Release và báo cáo', link: '/guides/release' },
        { text: 'Knowledge store và config team', link: '/guides/knowledge-and-config' },
        { text: 'Quan sát và an toàn', link: '/guides/observe-and-safety' },
        { text: 'Kiểm thử UI', link: '/workflows/ui-testing' },
        { text: 'Nâng cấp kit', link: '/getting-started/upgrade' } ] },
      { text: 'Tham chiếu', collapsed: false, items: [
        { text: 'CLI', link: '/reference/cli/' },
        { text: 'Skill', link: '/reference/skills/' },
        { text: 'Agent', link: '/reference/agents/' },
        { text: 'Hook', link: '/reference/hooks' } ] },
    ],
  },
})
