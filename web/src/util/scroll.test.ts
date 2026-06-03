import { describe, expect, it } from 'vitest'
import { isPinnedToBottom } from './scroll'

describe('isPinnedToBottom', () => {
  it('is pinned when scrolled to the exact bottom', () => {
    // scrollTop at its max (scrollHeight - clientHeight) → distance 0.
    expect(isPinnedToBottom(800, 1000, 200)).toBe(true)
  })

  it('is pinned when within the slack threshold of the bottom', () => {
    // 20px from the bottom, default threshold 32 → still pinned.
    expect(isPinnedToBottom(780, 1000, 200)).toBe(true)
  })

  it('is not pinned when scrolled up past the threshold', () => {
    expect(isPinnedToBottom(100, 1000, 200)).toBe(false)
  })

  it('treats an empty / unscrollable container as pinned', () => {
    // happy-dom and freshly-mounted columns report 0/0/0 — first events
    // should stick to the tail rather than be treated as "scrolled up".
    expect(isPinnedToBottom(0, 0, 0)).toBe(true)
  })

  it('honors a custom threshold', () => {
    expect(isPinnedToBottom(780, 1000, 200, 10)).toBe(false)
    expect(isPinnedToBottom(795, 1000, 200, 10)).toBe(true)
  })
})
