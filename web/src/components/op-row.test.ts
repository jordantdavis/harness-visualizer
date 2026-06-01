import { describe, expect, it } from 'vitest'
import './op-row'
import type { OpRow } from './op-row'

describe('cchv-op-row', () => {
  it('renders tool, target, a success glyph, and a duration', async () => {
    const el = document.createElement('cchv-op-row') as OpRow
    el.op = {
      id: 'a',
      tool: 'Edit',
      status: 'success',
      started_at: '',
      duration: 200_000_000, // 200ms in ns
      target: 'x.go',
      seq: 1,
    }
    document.body.append(el)
    await el.updateComplete
    const text = el.shadowRoot!.textContent ?? ''
    expect(text).toContain('Edit')
    expect(text).toContain('x.go')
    expect(text).toContain('✔')
    expect(text).toContain('200ms')
    el.remove()
  })
})
