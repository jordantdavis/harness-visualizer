import { describe, expect, it, beforeEach, vi } from 'vitest'
import { loadHooks, _resetHookCache } from '../api/hooks'
import type { LaneEvent } from '../api/types'
import './lane-event-row'

describe('hv-lane-event-row', () => {
  beforeEach(async () => {
    _resetHookCache()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => [
        { name: 'PermissionRequest', glyph: '🔒', label: 'Permission', lane: 'permission', severity: 'warn' },
      ],
    }))
    await loadHooks()
  })

  it('renders glyph + label + gist for a known hook', async () => {
    const el = document.createElement('hv-lane-event-row') as any
    el.event = {
      id: 'e1', hook_event: 'PermissionRequest', lane: 'permission',
      gist: 'Bash: ls', severity: 'warn', at: '2026-06-01T12:00:00Z', seq: 1,
    } satisfies LaneEvent
    document.body.appendChild(el)
    await el.updateComplete

    const text = el.shadowRoot!.textContent ?? ''
    expect(text).toContain('🔒')
    expect(text).toContain('Permission')
    expect(text).toContain('Bash: ls')
    el.remove()
  })

  it('falls back to hook name as gist when gist is empty', async () => {
    const el = document.createElement('hv-lane-event-row') as any
    el.event = {
      id: 'e2', hook_event: 'PermissionRequest', lane: 'permission',
      gist: '', severity: 'warn', at: '2026-06-01T12:00:00Z', seq: 2,
    }
    document.body.appendChild(el)
    await el.updateComplete

    const text = el.shadowRoot!.textContent ?? ''
    expect(text).toContain('PermissionRequest')
    el.remove()
  })

  it('applies dim class for severity=dim', async () => {
    // Severity comes from the event itself; the CSS rule only needs the
    // sev-dim class on the row. Hook name need not be a real registry entry.
    const el = document.createElement('hv-lane-event-row') as any
    el.event = {
      id: 'e3', hook_event: 'FakeDimHook', lane: 'unknown',
      gist: 'hello', severity: 'dim', at: '2026-06-01T12:00:00Z', seq: 3,
    }
    document.body.appendChild(el)
    await el.updateComplete

    const host = el.shadowRoot!.querySelector('.row')
    expect(host?.className).toContain('sev-dim')
    el.remove()
  })
})
