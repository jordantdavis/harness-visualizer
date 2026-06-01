/** A debounced function with a `cancel()` to drop a pending trailing call. */
export type Debounced<T extends (...args: never[]) => void> = T & { cancel(): void }

export function debounce<T extends (...args: never[]) => void>(
  fn: T,
  ms: number,
): Debounced<T> {
  let handle: ReturnType<typeof setTimeout> | undefined
  const wrapped = ((...args: Parameters<T>) => {
    if (handle) clearTimeout(handle)
    handle = setTimeout(() => {
      handle = undefined
      fn(...args)
    }, ms)
  }) as Debounced<T>
  wrapped.cancel = () => {
    if (handle) clearTimeout(handle)
    handle = undefined
  }
  return wrapped
}

/**
 * StreamController wraps an EventSource on /stream. Each frame is a nudge: it
 * (debounced) invokes onNudge, which the caller wires to a full timeline
 * refetch. The browser never parses event payloads for derivation.
 */
export class StreamController {
  private es?: EventSource
  private readonly nudge: Debounced<() => void>

  constructor(onNudge: () => void, debounceMs = 150) {
    this.nudge = debounce(onNudge, debounceMs)
  }

  connect(sessionId: string): void {
    this.disconnect()
    this.es = new EventSource(`/stream?session=${encodeURIComponent(sessionId)}`)
    this.es.onmessage = () => this.nudge()
  }

  disconnect(): void {
    this.nudge.cancel()
    this.es?.close()
    this.es = undefined
  }
}
