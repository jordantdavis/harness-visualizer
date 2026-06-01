import { describe, expect, it } from 'vitest'
import './diff-view'
import type { DiffView } from './diff-view'

describe('cchv-diff-view', () => {
  it('renders one del and one add line', async () => {
    const el = document.createElement('cchv-diff-view') as DiffView
    el.diff = [
      { kind: 'context', text: 'a' },
      { kind: 'del', text: 'b' },
      { kind: 'add', text: 'B' },
    ]
    document.body.append(el)
    await el.updateComplete
    expect(el.shadowRoot!.querySelectorAll('.del').length).toBe(1)
    const adds = el.shadowRoot!.querySelectorAll('.add')
    expect(adds.length).toBe(1)
    expect(adds[0].textContent).toContain('B')
    el.remove()
  })
})
