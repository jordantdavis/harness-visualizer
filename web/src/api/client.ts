import type { OperationDetail, SessionInfo, TimelineItem } from './types'

async function getJSON<T>(url: string): Promise<T> {
  const resp = await fetch(url)
  if (!resp.ok) {
    throw new Error(`${url}: HTTP ${resp.status}`)
  }
  return (await resp.json()) as T
}

export const api = {
  sessions: () => getJSON<SessionInfo[]>('/api/sessions'),
  timeline: (id: string, after = 0) =>
    getJSON<TimelineItem[]>(
      `/api/sessions/${encodeURIComponent(id)}/timeline?after=${after}`,
    ),
  operation: (id: string, opId: string) =>
    getJSON<OperationDetail>(
      `/api/sessions/${encodeURIComponent(id)}/operations/${encodeURIComponent(opId)}`,
    ),
  deleteSession: async (id: string): Promise<void> => {
    const url = `/api/sessions/${encodeURIComponent(id)}`
    const resp = await fetch(url, { method: 'DELETE' })
    if (!resp.ok) {
      throw new Error(`${url}: HTTP ${resp.status}`)
    }
  },
}
