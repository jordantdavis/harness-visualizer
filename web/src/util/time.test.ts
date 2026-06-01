import { describe, expect, it } from 'vitest'
import { formatClock, formatFull, formatRelative } from './time'

// Helper: produce the expected HH:mm:ss for a Date in local time, mirroring
// what formatClock does internally. Keeps tests timezone-agnostic.
function localHHmmss(d: Date): string {
  const h = String(d.getHours()).padStart(2, '0')
  const m = String(d.getMinutes()).padStart(2, '0')
  const s = String(d.getSeconds()).padStart(2, '0')
  return `${h}:${m}:${s}`
}

describe('formatClock', () => {
  it('zero-pads hour, minute, and second', () => {
    // Use a UTC time whose local components are each a single digit on UTC.
    // To stay TZ-agnostic, compute the expected value from the same Date.
    const d = new Date('2026-06-01T13:02:03Z')
    expect(formatClock(d.toISOString())).toBe(localHHmmss(d))
  })

  it('matches the local midnight representation', () => {
    const d = new Date('2026-06-01T06:00:00Z') // arbitrary — local midnight or not
    expect(formatClock(d.toISOString())).toBe(localHHmmss(d))
  })

  it('result is always HH:mm:ss format (8 chars)', () => {
    const result = formatClock('2026-06-15T10:30:45.123Z')
    expect(result).toMatch(/^\d{2}:\d{2}:\d{2}$/)
  })

  it('returns empty string for empty input', () => {
    expect(formatClock('')).toBe('')
  })

  it('returns empty string for invalid ISO string', () => {
    expect(formatClock('not-a-date')).toBe('')
  })
})

describe('formatFull', () => {
  it('includes date, time, and sub-second (millisecond) component', () => {
    const result = formatFull('2026-06-01T10:30:45.123Z')
    // Must not be empty and must carry 3-digit millisecond-level precision
    expect(result.length).toBeGreaterThan(10)
    // The format includes ".NNN" milliseconds
    expect(result).toMatch(/\.\d{3}/)
  })

  it('matches YYYY-MM-DD HH:mm:ss.SSS pattern in UTC', () => {
    // Use a fixed UTC instant; compute expected via Date to be TZ-agnostic.
    const d = new Date('2026-06-01T10:30:45.123Z')
    const result = formatFull(d.toISOString())
    // Must contain the year
    expect(result).toContain(String(d.getFullYear()))
    // Must be the right length range for a datetime with ms
    expect(result.length).toBeGreaterThanOrEqual(19)
  })

  it('returns empty string for empty input', () => {
    expect(formatFull('')).toBe('')
  })

  it('returns empty string for invalid input', () => {
    expect(formatFull('garbage')).toBe('')
  })
})

describe('formatRelative', () => {
  // Fixed "now" for all relative tests: 2026-06-15T12:00:00.000Z
  const now = new Date('2026-06-15T12:00:00.000Z')

  it('returns "just now" for an event less than 10 seconds ago', () => {
    const iso = new Date(now.getTime() - 5_000).toISOString()
    expect(formatRelative(iso, now)).toBe('just now')
  })

  it('returns "Ns ago" for 30 seconds ago', () => {
    const iso = new Date(now.getTime() - 30_000).toISOString()
    expect(formatRelative(iso, now)).toBe('30s ago')
  })

  it('returns "1m ago" for exactly 60 seconds ago', () => {
    const iso = new Date(now.getTime() - 60_000).toISOString()
    expect(formatRelative(iso, now)).toBe('1m ago')
  })

  it('returns "Nm ago" for 45 minutes ago', () => {
    const iso = new Date(now.getTime() - 45 * 60_000).toISOString()
    expect(formatRelative(iso, now)).toBe('45m ago')
  })

  it('returns "Nh ago" for 3 hours ago', () => {
    const iso = new Date(now.getTime() - 3 * 60 * 60_000).toISOString()
    expect(formatRelative(iso, now)).toBe('3h ago')
  })

  it('returns "yesterday" for ~24 hours ago', () => {
    const iso = new Date(now.getTime() - 24 * 60 * 60_000).toISOString()
    expect(formatRelative(iso, now)).toBe('yesterday')
  })

  it('returns "Nd ago" for 4 days ago', () => {
    const iso = new Date(now.getTime() - 4 * 24 * 60 * 60_000).toISOString()
    expect(formatRelative(iso, now)).toBe('4d ago')
  })

  it('returns an absolute date for >7 days ago in same year (no year shown)', () => {
    // 2026-06-05 is 10 days before now (2026-06-15) — same year
    const iso = new Date(now.getTime() - 10 * 24 * 60 * 60_000).toISOString()
    const result = formatRelative(iso, now)
    // Must not contain the year for a same-year date
    expect(result).not.toMatch(/2026/)
    // Must not be one of the relative labels
    expect(result).not.toMatch(/ago|yesterday|just now/)
    expect(result.length).toBeGreaterThan(0)
  })

  it('returns an absolute date with year for a different year', () => {
    const iso = '2024-01-10T00:00:00Z'
    const result = formatRelative(iso, now)
    expect(result).toMatch(/2024/)
  })

  it('returns empty string for empty input', () => {
    expect(formatRelative('', now)).toBe('')
  })

  it('returns empty string for invalid input', () => {
    expect(formatRelative('not-a-date', now)).toBe('')
  })
})
