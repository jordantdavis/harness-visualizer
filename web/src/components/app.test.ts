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
})
