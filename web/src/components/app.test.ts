import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import './app'
import type { App } from './app'

class FakeEventSource {
  onmessage: ((e: MessageEvent) => void) | null = null
  close() {}
}

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
})
