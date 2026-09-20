// Data loader: đọc GitHub Releases lúc build, nên trang changelog luôn khớp release mới nhất
// mà không ai phải sửa tay. Site rebuild khi `main` có push; goreleaser tự push commit cask/scoop
// lên `main` sau mỗi release, nên mỗi release kéo theo một lần rebuild đã thấy chính nó.
// Body release do goreleaser sinh, mỗi dòng `* <sha> <subject>`; lọc merge/cask/scoop, nhóm theo type.
const REPO = 'ZenifyAIContactCenter/zenify-kit'
const API = `https://api.github.com/repos/${REPO}/releases?per_page=30`

export interface ChangeLine { sha: string; short: string; subject: string; url: string }
export interface Release {
  tag: string
  date: string
  url: string
  groups: { title: string; lines: ChangeLine[] }[]
}
export interface ChangelogData { releases: Release[]; error: string | null; fetchedAt: string }

const GROUPS: [RegExp, string][] = [
  [/^feat(\(|:|!)/, 'Tính năng'],
  [/^fix(\(|:|!)/, 'Sửa lỗi'],
  [/^docs(\(|:|!)/, 'Tài liệu'],
]
const SKIP = /^(Merge (pull request|branch)|Brew cask update|Scoop update)/

function parseBody(body: string): Release['groups'] {
  const buckets = new Map<string, ChangeLine[]>()
  for (const raw of body.split('\n')) {
    const m = raw.match(/^\*\s+([0-9a-f]{7,40})\s+(.+)$/)
    if (!m) continue
    const [, sha, subject] = m
    if (SKIP.test(subject)) continue
    const title = GROUPS.find(([re]) => re.test(subject))?.[1] ?? 'Khác'
    const list = buckets.get(title) ?? []
    list.push({ sha, short: sha.slice(0, 7), subject, url: `https://github.com/${REPO}/commit/${sha}` })
    buckets.set(title, list)
  }
  const order = ['Tính năng', 'Sửa lỗi', 'Tài liệu', 'Khác']
  return order.filter((t) => buckets.has(t)).map((t) => ({ title: t, lines: buckets.get(t)! }))
}

export default {
  async load(): Promise<ChangelogData> {
    const fetchedAt = new Date().toISOString()
    const headers: Record<string, string> = { Accept: 'application/vnd.github+json' }
    if (process.env.GITHUB_TOKEN) headers.Authorization = `Bearer ${process.env.GITHUB_TOKEN}`
    try {
      const res = await fetch(API, { headers })
      if (!res.ok) throw new Error(`GitHub API ${res.status} ${res.statusText}`)
      const json = (await res.json()) as any[]
      const releases = json
        .filter((r) => !r.draft && !r.prerelease)
        .map((r) => ({
          tag: r.tag_name as string,
          date: String(r.published_at).slice(0, 10),
          url: r.html_url as string,
          groups: parseBody(String(r.body ?? '')),
        }))
      return { releases, error: null, fetchedAt }
    } catch (e) {
      const error = e instanceof Error ? e.message : String(e)
      // CI phải thấy lỗi thật; build tay không mạng vẫn ra site, trang này báo lý do.
      if (process.env.CI) throw new Error(`changelog.data.ts: ${error}`)
      console.warn(`[changelog] không tải được GitHub Releases: ${error}`)
      return { releases: [], error, fetchedAt }
    }
  },
}
