/** Keep positive quota below 1% distinguishable from exhaustion. */
export function formatRemainingPercent(value: number | null | undefined): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '—'
  const percent = Math.max(0, Math.min(100, value))
  return percent > 0 && percent < 1 ? '<1%' : `${Math.floor(percent)}%`
}
