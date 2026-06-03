import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import './app'
import type { App } from './app'

class FakeEventSource {
  onmessage: ((e: MessageEvent) => void) | null = null
  close() {}
}

/** Settle the async selection handlers + Lit's render queue. */
async function flush(el: App) {
  for (let i = 0; i < 5; i++) {
    await Promise.resolve()
    await el.updateComplete
  }
}

/** Route fetch by URL so selection drives real timeline/operation responses. */
function routedFetch(timeline: unknown[], detail: unknown) {
  return vi.fn((input: RequestInfo | URL) => {
    const url = String(input)
    let body: unknown = []
    if (url.includes('/operations/')) body = detail
    else if (url.includes('/timeline')) body = timeline
    else if (url.includes('/api/sessions')) body = []
    return Promise.resolve(new Response(JSON.stringify(body), { status: 200 }))
  })
}

const OP_ITEM = {
  kind: 'operation',
  at: '2026-01-01T00:00:00Z',
  seq: 1,
  op: { id: 'op1', tool: 'Read', status: 'success', started_at: '', duration: 0, target: '', seq: 1 },
}
const OP_DETAIL = { id: 'op1', tool: 'Read', detail_kind: 'generic' }

describe('hv-app', () => {
  beforeEach(() => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('[]', { status: 200 }))
    vi.stubGlobal('EventSource', FakeEventSource)
  })
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('renders the three panes', async () => {
    const el = document.createElement('hv-app') as App
    document.body.append(el)
    await el.updateComplete
    const root = el.shadowRoot!
    expect(root.querySelector('hv-session-list')).toBeTruthy()
    expect(root.querySelector('hv-timeline')).toBeTruthy()
    expect(root.querySelector('hv-inspector')).toBeTruthy()
    el.remove()
  })

  it('uses widened inspector column (520px)', async () => {
    const el = document.createElement('hv-app') as any
    document.body.appendChild(el)
    await el.updateComplete

    // The styles are static; assert against the constructor's static styles.
    const css = (el.constructor as typeof HTMLElement & { styles: unknown }).styles
    const cssText = Array.isArray(css) ? css.map((c) => String(c)).join('\n') : String(css)
    expect(cssText).toContain('520px')
    // And does NOT still contain the old 380px width for the inspector.
    expect(cssText).not.toMatch(/240px 1fr 380px/)

    el.remove()
  })

  it('gives each column exactly one scroll owner (the body)', async () => {
    const el = document.createElement('hv-app') as any
    document.body.appendChild(el)
    await el.updateComplete
    const css = (el.constructor as typeof HTMLElement & { styles: unknown }).styles
    const cssText = Array.isArray(css) ? css.map((c) => String(c)).join('\n') : String(css)

    // The page itself never scrolls; the grid is height-bounded.
    expect(cssText).toMatch(/:host\s*{[^}]*overflow:\s*hidden/)
    expect(cssText).toMatch(/\.panes\s*{[^}]*overflow:\s*hidden/)
    // Only the per-column body scrolls — exactly one scroll owner.
    expect((cssText.match(/overflow(-y)?:\s*auto/g) ?? []).length).toBe(1)
    expect(cssText).toMatch(/\.body\s*{[^}]*overflow(-y)?:\s*auto/)
    // Scrollbar gutter is reserved so content doesn't shift horizontally.
    expect(cssText).toContain('scrollbar-gutter: stable')
    el.remove()
  })

  it('resets timeline and inspector scroll to the top when a session is selected', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(routedFetch([OP_ITEM], OP_DETAIL) as any)
    const el = document.createElement('hv-app') as App
    document.body.append(el)
    await flush(el)

    const root = el.shadowRoot!
    const tl = root.querySelector('.body-timeline') as HTMLElement
    const insp = root.querySelector('.body-inspector') as HTMLElement
    tl.scrollTop = 50
    insp.scrollTop = 50

    el.dispatchEvent(new CustomEvent('select-session', { detail: 's1' }))
    await flush(el)

    expect(tl.scrollTop).toBe(0)
    expect(insp.scrollTop).toBe(0)
    el.remove()
  })

  it('resets inspector scroll to the top when a different operation is selected', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(routedFetch([OP_ITEM], OP_DETAIL) as any)
    const el = document.createElement('hv-app') as App
    document.body.append(el)
    el.dispatchEvent(new CustomEvent('select-session', { detail: 's1' }))
    await flush(el)

    const insp = el.shadowRoot!.querySelector('.body-inspector') as HTMLElement
    insp.scrollTop = 80
    el.dispatchEvent(new CustomEvent('select-op', { detail: 'op1' }))
    await flush(el)

    expect(insp.scrollTop).toBe(0)
    el.remove()
  })

  it('does not reset inspector scroll when the same operation is re-selected', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(routedFetch([OP_ITEM], OP_DETAIL) as any)
    const el = document.createElement('hv-app') as App
    document.body.append(el)
    el.dispatchEvent(new CustomEvent('select-session', { detail: 's1' }))
    await flush(el)
    el.dispatchEvent(new CustomEvent('select-op', { detail: 'op1' }))
    await flush(el)

    const insp = el.shadowRoot!.querySelector('.body-inspector') as HTMLElement
    insp.scrollTop = 80
    // Re-selecting the already-selected op must not yank the reader's scroll.
    el.dispatchEvent(new CustomEvent('select-op', { detail: 'op1' }))
    await flush(el)

    expect(insp.scrollTop).toBe(80)
    el.remove()
  })
})
