import type { HookMeta, Severity } from './types'

let cache: Map<string, HookMeta> | null = null
let inflight: Promise<void> | null = null

const FALLBACK_SEVERITY: Severity = 'info'

/** Internal: clear the cache. Tests only. */
export function _resetHookCache(): void {
  cache = null
  inflight = null
}

/** Fetch the registry once and cache it. Subsequent calls are no-ops. On
 *  fetch failure, cache is set to empty Map so lookups still work via the
 *  generic fallback. */
export async function loadHooks(): Promise<void> {
  if (cache) return
  if (inflight) return inflight
  inflight = (async () => {
    try {
      const res = await fetch('/api/hooks')
      if (!res.ok) {
        cache = new Map()
        return
      }
      const list = (await res.json()) as HookMeta[]
      cache = new Map(list.map((h) => [h.name, h]))
    } catch {
      cache = new Map()
    } finally {
      inflight = null
    }
  })()
  return inflight
}

/** Lookup metadata for a hook event name. Returns a generic fallback when
 *  the hook is unknown, so unrecognized future hooks still render a row. */
export function lookupHook(name: string): HookMeta {
  const found = cache?.get(name)
  if (found) return found
  return {
    name,
    glyph: '·',
    label: name,
    lane: 'unknown',
    severity: FALLBACK_SEVERITY,
  }
}
