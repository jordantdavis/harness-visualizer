import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './client'

afterEach(() => vi.restoreAllMocks())

describe('api client', () => {
  it('fetches sessions from /api/sessions', async () => {
    const data = [{ id: 's1', event_count: 1, last_seq: 1, mod_time: '' }]
    const spy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(JSON.stringify(data), { status: 200 }))
    expect(await api.sessions()).toEqual(data)
    expect(spy).toHaveBeenCalledWith('/api/sessions')
  })

  it('builds the timeline url with an after cursor and encodes the id', async () => {
    const spy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response('[]', { status: 200 }))
    await api.timeline('s 1', 5)
    expect(spy).toHaveBeenCalledWith('/api/sessions/s%201/timeline?after=5')
  })

  it('throws on a non-ok response', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('nope', { status: 500 }))
    await expect(api.operation('s', 'x')).rejects.toThrow()
  })

  it('deletes a session with DELETE and an encoded id', async () => {
    const spy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(null, { status: 204 }))
    await expect(api.deleteSession('s 1')).resolves.toBeUndefined()
    expect(spy).toHaveBeenCalledWith('/api/sessions/s%201', { method: 'DELETE' })
  })

  it('throws when deleteSession gets a non-ok response', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('boom', { status: 500 }))
    await expect(api.deleteSession('s1')).rejects.toThrow()
  })
})
