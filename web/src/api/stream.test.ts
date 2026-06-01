import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { debounce } from './stream'

beforeEach(() => vi.useFakeTimers())
afterEach(() => vi.useRealTimers())

describe('debounce', () => {
  it('coalesces rapid calls into a single trailing invocation', () => {
    const fn = vi.fn()
    const d = debounce(fn, 150)
    d()
    d()
    d()
    expect(fn).not.toHaveBeenCalled()
    vi.advanceTimersByTime(150)
    expect(fn).toHaveBeenCalledTimes(1)
  })

  it('cancel() prevents a pending invocation', () => {
    const fn = vi.fn()
    const d = debounce(fn, 150)
    d()
    d.cancel()
    vi.advanceTimersByTime(200)
    expect(fn).not.toHaveBeenCalled()
  })
})
