import type { NonstopStats } from '../types'

export type { NonstopStats }

// FR-5.5: the JSON `zenify nonstop stats --json` prints. Throws on anything else.
export function parseStats(stdout: string): NonstopStats {
  const data = JSON.parse(stdout) as Partial<NonstopStats> | null
  if (!data || typeof data.missing !== 'boolean' || !data.scale || !data.today || !data.week || !Array.isArray(data.rules) || !data.tiers || typeof data.today.reviewMinutes !== 'number') {
    throw new Error('đầu ra không đúng dạng thống kê')
  }
  return data as NonstopStats
}

export type StatsLine = { text: string; warn?: boolean; cut?: boolean }

// FR-3.10: the lines of the THỐNG KÊ section. `doneToday` = ids of the ledger's done tasks of
// today (empty when the ledger is not today's), checked against the ids stats saw.
export function statsLines(s: NonstopStats, doneToday: string[]): StatsLine[] {
  if (s.missing) return [{ text: 'chưa có estimates.md' }]
  const { scale: c, today: t, week: w } = s
  const lines: StatsLine[] = []
  const bounds = `S<${c.S}m · M<${c.M}m · L<${c.L}m`
  lines.push({
    text: c.source === 'learned' ? `thang (học từ ${c.n} task): ${bounds}` : `thang (mặc định, ${c.n}/20 task): ${bounds}`,
  })
  lines.push({ text: `hôm nay: đúng ${t.hit}/${t.total} · ${t.over} cao · ${t.under} thấp · ${t.points} điểm` })
  const share = w.reviewShare === null ? '' : ` (${Math.round(w.reviewShare * 100)}% thời gian)`
  lines.push({ text: `review: ${t.reviewMinutes} phút${share}` })
  lines.push({ text: `tier 7 ngày: T1 ${s.tiers.T1} · T2 ${s.tiers.T2} · T3 ${s.tiers.T3}` })
  const rate = w.hitRate === null ? '—' : `${Math.round(w.hitRate * 100)}%`
  const ppd = w.pointsPerDay === null ? '—' : w.pointsPerDay.toFixed(1)
  lines.push({ text: `7 ngày: đúng ${rate} · ${ppd} điểm/ngày` })
  for (const r of s.rules) lines.push({ text: r })
  const unrecorded = doneToday.filter(id => !t.ids.includes(id))
  if (unrecorded.length) {
    lines.push({ text: `⚠ ${unrecorded.length} task done hôm nay chưa ghi estimate: ${unrecorded.join(', ')}`, warn: true, cut: true })
  }
  if (s.malformed > 0) lines.push({ text: `⚠ ${s.malformed} dòng estimates sai format`, warn: true })
  return lines
}
