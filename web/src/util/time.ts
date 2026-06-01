/**
 * Lightweight, dependency-free timestamp helpers for the hv web UI.
 * All functions accept ISO 8601 strings (as emitted by Go's time.Time JSON
 * marshalling) and return '' for any invalid or empty input — never throw.
 */

/**
 * formatClock — local wall-clock time as HH:mm:ss (zero-padded).
 *
 * Returns '' for empty or unparseable input.
 */
export function formatClock(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  const h = String(d.getHours()).padStart(2, '0')
  const m = String(d.getMinutes()).padStart(2, '0')
  const s = String(d.getSeconds()).padStart(2, '0')
  return `${h}:${m}:${s}`
}

/**
 * formatFull — local datetime as "YYYY-MM-DD HH:mm:ss.SSS ±HH:MM".
 *
 * Provides a stable, sortable, human-readable timestamp suitable for tooltips.
 * The timezone offset at the end is the system's local offset (e.g. +00:00,
 * -05:00). Returns '' for empty or unparseable input.
 */
export function formatFull(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''

  const YYYY = d.getFullYear()
  const MM = String(d.getMonth() + 1).padStart(2, '0')
  const DD = String(d.getDate()).padStart(2, '0')
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  const ss = String(d.getSeconds()).padStart(2, '0')
  const SSS = String(d.getMilliseconds()).padStart(3, '0')

  // Build the UTC-offset string, e.g. "+05:30" or "-07:00".
  const offsetMin = -d.getTimezoneOffset() // getTimezoneOffset returns -(local - UTC)
  const sign = offsetMin >= 0 ? '+' : '-'
  const absMin = Math.abs(offsetMin)
  const offH = String(Math.floor(absMin / 60)).padStart(2, '0')
  const offM = String(absMin % 60).padStart(2, '0')

  return `${YYYY}-${MM}-${DD} ${hh}:${mm}:${ss}.${SSS} ${sign}${offH}:${offM}`
}

/**
 * formatRelative — human-friendly relative label for an ISO timestamp.
 *
 * Buckets (compared to `now`):
 *   < 10s        → "just now"
 *   < 60s        → "Ns ago"
 *   < 60m        → "Nm ago"
 *   < 24h        → "Nh ago"
 *   < 48h        → "yesterday"
 *   < 7d         → "Nd ago"
 *   same year    → abbreviated month + day, e.g. "Jun 5"
 *   other year   → "Jun 5, 2024"
 *
 * Returns '' for empty or unparseable input.
 */
export function formatRelative(iso: string, now: Date): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''

  const diffMs = now.getTime() - d.getTime()
  const diffS = Math.floor(diffMs / 1000)
  const diffM = Math.floor(diffMs / 60_000)
  const diffH = Math.floor(diffMs / 3_600_000)
  const diffD = Math.floor(diffMs / 86_400_000)

  if (diffS < 10) return 'just now'
  if (diffS < 60) return `${diffS}s ago`
  if (diffM < 60) return `${diffM}m ago`
  if (diffH < 24) return `${diffH}h ago`
  if (diffD < 2) return 'yesterday'
  if (diffD < 7) return `${diffD}d ago`

  // Absolute date — same year omits the year.
  const sameYear = d.getFullYear() === now.getFullYear()
  const opts: Intl.DateTimeFormatOptions = sameYear
    ? { month: 'short', day: 'numeric' }
    : { month: 'short', day: 'numeric', year: 'numeric' }
  return new Intl.DateTimeFormat(undefined, opts).format(d)
}
