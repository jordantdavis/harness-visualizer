import { describe, expect, it, beforeEach, vi } from 'vitest'
import { loadHooks, lookupHook, _resetHookCache } from './hooks'

describe('hooks registry', () => {
  beforeEach(() => {
    _resetHookCache()
    vi.restoreAllMocks()
  })

  it('fetches and caches hooks on first call', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => [
        { name: 'PermissionRequest', glyph: '🔒', label: 'Permission', lane: 'permission', severity: 'warn' },
      ],
    })
    vi.stubGlobal('fetch', fetchMock)

    await loadHooks()
    await loadHooks() // second call should not refetch

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledWith('/api/hooks')
  })

  it('looks up a known hook', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => [
        { name: 'PermissionRequest', glyph: '🔒', label: 'Permission', lane: 'permission', severity: 'warn' },
      ],
    }))

    await loadHooks()
    const meta = lookupHook('PermissionRequest')
    expect(meta?.glyph).toBe('🔒')
    expect(meta?.label).toBe('Permission')
  })

  it('returns a generic fallback for unknown hooks', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => [] }))

    await loadHooks()
    const meta = lookupHook('SomeFutureHook')
    expect(meta.name).toBe('SomeFutureHook')
    expect(meta.glyph).toBe('·')
    expect(meta.lane).toBe('unknown')
  })

  it('survives fetch failure (registry stays empty, fallback works)', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 500 }))

    await loadHooks() // should not throw
    const meta = lookupHook('PermissionRequest')
    expect(meta.glyph).toBe('·') // fallback because registry empty
  })
})
