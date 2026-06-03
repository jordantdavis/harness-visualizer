export interface SessionInfo {
  id: string
  event_count: number
  last_seq: number
  mod_time: string
  /** ISO 8601 timestamp of the first captured event. Empty string when unknown. */
  started_at: string
  /**
   * ISO 8601 timestamp of the last captured event — the server-side sort key
   * ("most recent activity, newest first"). Empty string when unknown.
   */
  last_activity: string
  cwd?: string
  title?: string
}

export type Status = 'running' | 'success' | 'error' | 'neutral'
export type Severity = 'info' | 'warn' | 'error' | 'dim'

export interface Operation {
  id: string
  tool: string
  status: Status
  started_at: string
  duration: number // nanoseconds (Go time.Duration)
  target: string
  seq: number
}

export interface Turn {
  role: 'user' | 'assistant'
  text: string
  thinking?: string
  tool_refs?: string[]
  at: string
}

export interface LaneEvent {
  id: string
  hook_event: string
  lane: string
  gist: string
  severity: Severity
  raw?: unknown
  at: string
  seq: number
}

export interface TimelineItem {
  kind: 'operation' | 'turn' | 'event'
  at: string
  seq: number
  op?: Operation
  turn?: Turn
  event?: LaneEvent
}

export interface DiffOp {
  kind: 'context' | 'del' | 'add'
  text: string
}

export interface OperationDetail {
  id: string
  tool: string
  detail_kind: 'diff' | 'output' | 'generic'
  file_path?: string
  diff?: DiffOp[]
  command?: string
  output?: string
  exit_code?: number
  raw_pre?: unknown
  raw_post?: unknown
}

export interface HookMeta {
  name: string
  glyph: string
  label: string
  lane: string
  severity: Severity
}
