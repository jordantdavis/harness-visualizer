import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import './app'
import type { App } from './app'

class FakeEventSource {
  onmessage: ((e: MessageEvent) => void) | null = null
  close() {}
}

describe('cchv-app', () => {
  beforeEach(() => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('[]', { status: 200 }))
    vi.stubGlobal('EventSource', FakeEventSource)
  })
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('renders the three panes', async () => {
    const el = document.createElement('cchv-app') as App
    document.body.append(el)
    await el.updateComplete
    const root = el.shadowRoot!
    expect(root.querySelector('cchv-session-list')).toBeTruthy()
    expect(root.querySelector('cchv-timeline')).toBeTruthy()
    expect(root.querySelector('cchv-inspector')).toBeTruthy()
    el.remove()
  })
})
