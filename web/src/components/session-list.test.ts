import { afterEach, describe, expect, it, vi } from 'vitest'
import './session-list'
import type { SessionInfo } from '../api/types'
import type { SessionList } from './session-list'

afterEach(() => vi.restoreAllMocks())

const sessions: SessionInfo[] = [
  { id: 'abc123', event_count: 2, last_seq: 2, mod_time: '', started_at: '', last_activity: '' },
]

async function mount(): Promise<SessionList> {
  const el = document.createElement('hv-session-list') as SessionList
  el.sessions = sessions
  document.body.append(el)
  await el.updateComplete
  return el
}

describe('hv-session-list delete', () => {
  it('dispatches delete-session (and not select-session) on confirm', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const el = await mount()

    let deletedId = ''
    let selected = false
    el.addEventListener('delete-session', (e) => {
      deletedId = (e as CustomEvent<string>).detail
    })
    el.addEventListener('select-session', () => {
      selected = true
    })

    const btn = el.shadowRoot!.querySelector('.del') as HTMLButtonElement
    expect(btn).toBeTruthy()
    expect(btn.getAttribute('aria-label')).toContain('Delete')
    btn.click()

    expect(deletedId).toBe('abc123')
    expect(selected).toBe(false) // stopPropagation kept the row click from firing
  })

  it('does nothing when the confirm is declined', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    const el = await mount()

    let deleted = false
    el.addEventListener('delete-session', () => {
      deleted = true
    })

    const btn = el.shadowRoot!.querySelector('.del') as HTMLButtonElement
    btn.click()
    expect(deleted).toBe(false)
  })
})
