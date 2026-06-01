export interface SessionInfo {
  id: string
  event_count: number
  last_seq: number
  mod_time: string
  cwd?: string
  title?: string
}

export type Status = 'running' | 'success' | 'error' | 'neutral'

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

export interface TimelineItem {
  kind: 'operation' | 'turn'
  at: string
  seq: number
  op?: Operation
  turn?: Turn
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
