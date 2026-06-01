# Web UI Frontend — Implementation Plan (Plan 2 of 2)

## Status

**COMPLETE & browser-verified (2026-05-31).** All 12 tasks implemented via subagent-driven TDD on branch `feat/web-ui-frontend`, each gated by spec + code-quality review. `go build ./...`, `go vet ./...`, and `go test ./...` (all 10 packages) are green; `cd web && npm run build` (tsc + vite) is clean and `npm run test` passes (vitest, 8 tests). A live end-to-end check via the chrome-devtools MCP drove the **embedded production binary**: the three-pane UI rendered, the session list → timeline (✔/▶ glyphs) → inspector flow worked, the server-derived structured diff and Bash output/raw views rendered correctly, and the console was clean. `cchv serve` was verified to reuse an already-healthy daemon (no duplicate spawn) and dispatch the browser opener. A final whole-branch review found no Critical/Important issues; two minor follow-ups were applied (empty-`id` op guard; `postbuild` keeps a bare `npm run build` tree-clean). One deferred-by-design note: the `/timeline?after=` cursor is sent by the client but not yet applied server-side (full-refetch v1; seq-cursor pagination is a tracked fast-follow).

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Lit + Vite web UI (a dumb client of Plan 1's `/api`), embed the production bundle into the `cchv` binary, and add a `cchv serve` launcher — so a user can `make build && cchv serve` and see the three-pane Sessions │ Timeline │ Inspector view live.

**Architecture:** The browser never derives. It fetches server-derived `TimelineItem`/`OperationDetail` JSON from Plan 1's `/api`, subscribes to `/stream` SSE as a *nudge* (debounced refetch), and renders terminal-native Lit components. A controller component (`cchv-app`) owns all state; children are presentational and emit `CustomEvent`s. The Vite dev server proxies `/api` + `/stream` to the daemon (same-origin → no CORS). For production, Vite builds into `internal/web/dist`, which is `go:embed`'d and served at `/` by the daemon with SPA fallback. `cchv serve` ensures the daemon is up (reusing `internal/client`'s detach logic) and opens the browser.

**Tech Stack:** Lit 3 + TypeScript 5 + Vite 6, Vitest + happy-dom for tests. Go stdlib `embed`/`net/http`/`os/exec` for embed + launcher. No new Go dependencies.

**Spec:** `docs/superpowers/specs/2026-05-31-web-ui-pivot-design.md` (sections E, F, G + sequencing steps 4–5)

**Builds on:** `docs/superpowers/plans/2026-05-31-web-ui-backend-foundation.md` (Plan 1, complete). Wire contract: `GET /api/sessions`, `GET /api/sessions/{id}/timeline`, `GET /api/sessions/{id}/operations/{opID}`, `GET /stream?session={id}`.

---

## Scope & sequencing notes (read first)

- **This is Plan 2 of 2.** Plan 1 = backend (`internal/model`, `internal/source/claudecode`, `/api`) — complete on `main`. Plan 2 = frontend + embed + launcher (this document).
- **Thin vertical slice (spec §G).** In v1: session list, live interleaved timeline, inspector (real diff for Edit/Write, Bash/Read output, raw-JSON escape hatch), `cchv serve`, embedded prod build, mouse-driven. **Explicitly NOT in v1:** keyboard nav, filter/error-hop/folding, light theme, virtualization, syntax-highlighting grammars, Model B streaming, full E2E suite.
- **`go:embed` package lives at `internal/web/`** (not `web/`) so the embed pattern `all:dist` resolves to `internal/web/dist` without an illegal `../`. Vite's `outDir` is `../internal/web/dist`. Frontend *source* stays in `web/`.
- **Full-timeline replace, not incremental upsert (v1).** Plan 1's `/timeline` returns the whole session (correct pairing needs whole-history context). The client refetches the full timeline on each SSE nudge and replaces its list. "Upsert by `Operation.ID`" becomes relevant only when seq-cursor pagination lands (a fast-follow); doing a full replace now is correct and simpler (YAGNI).
- **`time.Duration` is nanoseconds on the wire.** Go marshals `Operation.Duration` as an int64 nanosecond count. The client converts to ms/s for display.

## File structure

| File | Responsibility |
|---|---|
| `web/package.json` | npm project: lit dep, vite/vitest/typescript/happy-dom devDeps, build/test scripts |
| `web/package-lock.json` | committed lockfile (so `npm ci` + a clean Makefile build are reproducible) |
| `web/tsconfig.json` | TS config tuned for Lit (`experimentalDecorators`, `useDefineForClassFields:false`) |
| `web/vite.config.ts` | dev proxy (`/api`,`/stream` → daemon), `build.outDir=../internal/web/dist`, vitest config |
| `web/index.html` | SPA entry: `<cchv-app>` + module script |
| `web/src/global.css` | terminal palette CSS custom properties on `:root` + base body styles |
| `web/src/main.ts` | bootstrap: import global.css + register `cchv-app` |
| `web/src/api/types.ts` | TS mirrors of the Go wire types (SessionInfo, Operation, Turn, TimelineItem, OperationDetail, DiffOp) |
| `web/src/api/client.ts` | `api.sessions/timeline/operation` fetch helpers |
| `web/src/api/stream.ts` | `debounce` + `StreamController` (EventSource nudge wrapper) |
| `web/src/components/op-row.ts` | `<cchv-op-row>` — one tool operation row |
| `web/src/components/turn-row.ts` | `<cchv-turn-row>` — one conversation turn row |
| `web/src/components/diff-view.ts` | `<cchv-diff-view>` — structured line diff |
| `web/src/components/code-view.ts` | `<cchv-code-view>` — Bash/Read command + output |
| `web/src/components/raw-view.ts` | `<cchv-raw-view>` — pretty-printed raw JSON escape hatch |
| `web/src/components/inspector.ts` | `<cchv-inspector>` — picks a subview from `OperationDetail` |
| `web/src/components/session-list.ts` | `<cchv-session-list>` — left pane |
| `web/src/components/timeline.ts` | `<cchv-timeline>` — center pane (op + turn rows) |
| `web/src/components/top-bar.ts` | `<cchv-top-bar>` — daemon status / live indicator |
| `web/src/components/app.ts` | `<cchv-app>` — controller: state, fetches, stream, layout |
| `internal/web/embed.go` | `//go:embed all:dist` + `Handler()` + testable `NewHandler(fs.FS)` SPA handler |
| `internal/web/dist/.gitkeep` | committed placeholder so a fresh checkout `go build`s before `make build` |
| `internal/daemon/daemon.go` | mount `web.Handler()` at `/` in `NewServer` (modify) |
| `internal/client/client.go` | add exported `EnsureDaemon()` (+ `daemonHealthy`) (modify) |
| `internal/serve/serve.go` | `cchv serve` launcher: ensure daemon + open browser |
| `cmd/cchv/main.go` | dispatch `serve` subcommand (modify) |
| `Makefile` | `build: web go-build`; `web`, `test`, `test-web`, `clean` targets |
| `.gitignore` | ignore `internal/web/dist/*` (keep `.gitkeep`) + `web/node_modules` (modify) |

`*.test.ts` files live beside their source; `*_test.go` beside theirs.

---

### Task 1: `web/` scaffold + Vite/Vitest tooling

**Files:**
- Create: `web/package.json`, `web/tsconfig.json`, `web/vite.config.ts`, `web/index.html`, `web/src/global.css`, `web/src/main.ts`
- Modify: `.gitignore`

- [ ] **Step 1: Create `web/package.json`**

```json
{
  "name": "cchv-web",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc --noEmit && vite build",
    "preview": "vite preview",
    "test": "vitest run"
  },
  "dependencies": {
    "lit": "^3.2.1"
  },
  "devDependencies": {
    "happy-dom": "^15.11.7",
    "typescript": "^5.7.2",
    "vite": "^6.0.5",
    "vitest": "^2.1.8"
  }
}
```

- [ ] **Step 2: Create `web/tsconfig.json`**

```json
{
  "compilerOptions": {
    "target": "ES2021",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "lib": ["ES2021", "DOM", "DOM.Iterable"],
    "experimentalDecorators": true,
    "useDefineForClassFields": false,
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "skipLibCheck": true,
    "isolatedModules": true,
    "types": ["vite/client"]
  },
  "include": ["src"]
}
```

- [ ] **Step 3: Create `web/vite.config.ts`**

`defineConfig` from `vitest/config` carries both Vite build options and the `test` block. `outDir` points outside the Vite root, so `emptyOutDir` must be explicit.

```ts
import { defineConfig } from 'vitest/config'

export default defineConfig({
  server: {
    proxy: {
      '/api': { target: 'http://127.0.0.1:7842', changeOrigin: true },
      '/stream': { target: 'http://127.0.0.1:7842', changeOrigin: true },
    },
  },
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
  },
  test: {
    environment: 'happy-dom',
  },
})
```

- [ ] **Step 4: Create `web/index.html`**

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>cchv</title>
  </head>
  <body>
    <cchv-app></cchv-app>
    <script type="module" src="/src/main.ts"></script>
  </body>
</html>
```

- [ ] **Step 5: Create `web/src/global.css`** (palette from `docs/tui-mockup.html`)

```css
:root {
  --bg: #11141a;
  --bg-pane: #11141a;
  --fg: #c9d1d9;
  --fg-dim: #768390;
  --fg-faint: #4a525c;
  --sel-bg: #1d2530;
  --sel-bg-foc: #23303f;
  --accent: #58a6ff;
  --green: #56d364;
  --red: #f85149;
  --yellow: #e3b341;
  --magenta: #bc8cff;
  --bar-bg: #161b22;
  --border: #2d333b;
  --mono: "SF Mono", "JetBrains Mono", "Cascadia Code", "Menlo", "DejaVu Sans Mono", monospace;
}

html, body {
  margin: 0;
  padding: 0;
  height: 100%;
  background: #05070a;
  color: var(--fg);
  font-family: var(--mono);
  font-size: 13px;
  line-height: 1.5;
  font-variant-ligatures: none;
}

cchv-app { display: block; height: 100vh; }
```

- [ ] **Step 6: Create `web/src/main.ts`** (temporary placeholder; replaced in Task 8)

```ts
import './global.css'

// Placeholder bootstrap so the scaffold builds. Task 8 replaces this with the
// real `import './components/app'`.
customElements.define(
  'cchv-app',
  class extends HTMLElement {
    connectedCallback() {
      this.textContent = 'cchv web — scaffold OK'
    }
  },
)
```

- [ ] **Step 7: Update `.gitignore`** — append these lines

```gitignore
# Web frontend
/web/node_modules/
/web/dist/
/internal/web/dist/*
!/internal/web/dist/.gitkeep
```

- [ ] **Step 8: Install deps and build**

Run:
```bash
cd web && npm install && npm run build
```
Expected: `npm install` writes `web/package-lock.json` and `web/node_modules`; `npm run build` type-checks clean and emits `internal/web/dist/index.html` + `internal/web/dist/assets/*`. (Vite prints a "outDir … is outside of project root" warning — expected and harmless.)

- [ ] **Step 9: Verify the build output exists**

Run: `ls internal/web/dist/`
Expected: `index.html` and an `assets/` directory.

- [ ] **Step 10: Commit** (source + lockfile only; `dist` is gitignored)

```bash
git add web/package.json web/package-lock.json web/tsconfig.json web/vite.config.ts web/index.html web/src/global.css web/src/main.ts .gitignore
git commit -m "feat(web): Lit + Vite scaffold with dev proxy and vitest"
```

---

### Task 2: API types + fetch client

**Files:**
- Create: `web/src/api/types.ts`, `web/src/api/client.ts`
- Test: `web/src/api/client.test.ts`

- [ ] **Step 1: Create `web/src/api/types.ts`** (mirror Plan 1's Go wire tags)

```ts
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
```

- [ ] **Step 2: Write the failing test** `web/src/api/client.test.ts`

```ts
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './client'

afterEach(() => vi.restoreAllMocks())

describe('api client', () => {
  it('fetches sessions', async () => {
    const data = [{ id: 's1', event_count: 1, last_seq: 1, mod_time: '' }]
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(data), { status: 200 }),
    )
    expect(await api.sessions()).toEqual(data)
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
})
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd web && npx vitest run src/api/client.test.ts`
Expected: FAIL — cannot resolve `./client`.

- [ ] **Step 4: Create `web/src/api/client.ts`**

```ts
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
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd web && npx vitest run src/api/client.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
git add web/src/api/types.ts web/src/api/client.ts web/src/api/client.test.ts
git commit -m "feat(web): api wire types + fetch client"
```

---

### Task 3: `debounce` + `StreamController`

**Files:**
- Create: `web/src/api/stream.ts`
- Test: `web/src/api/stream.test.ts`

The SSE frame is a *nudge*; the client debounces (~150ms) then refetches the timeline. `debounce` is the only piece worth unit-testing (EventSource is environment glue).

- [ ] **Step 1: Write the failing test** `web/src/api/stream.test.ts`

```ts
import { describe, expect, it, vi } from 'vitest'
import { debounce } from './stream'

describe('debounce', () => {
  it('coalesces rapid calls into a single trailing invocation', () => {
    vi.useFakeTimers()
    const fn = vi.fn()
    const d = debounce(fn, 150)
    d()
    d()
    d()
    expect(fn).not.toHaveBeenCalled()
    vi.advanceTimersByTime(150)
    expect(fn).toHaveBeenCalledTimes(1)
    vi.useRealTimers()
  })

  it('cancel() prevents a pending invocation', () => {
    vi.useFakeTimers()
    const fn = vi.fn()
    const d = debounce(fn, 150)
    d()
    d.cancel()
    vi.advanceTimersByTime(200)
    expect(fn).not.toHaveBeenCalled()
    vi.useRealTimers()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/api/stream.test.ts`
Expected: FAIL — cannot resolve `./stream`.

- [ ] **Step 3: Create `web/src/api/stream.ts`**

```ts
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/api/stream.test.ts`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/api/stream.ts web/src/api/stream.test.ts
git commit -m "feat(web): debounce + StreamController SSE nudge wrapper"
```

---

### Task 4: timeline row components (`op-row`, `turn-row`)

**Files:**
- Create: `web/src/components/op-row.ts`, `web/src/components/turn-row.ts`
- Test: `web/src/components/op-row.test.ts`

- [ ] **Step 1: Write the failing test** `web/src/components/op-row.test.ts`

```ts
import { describe, expect, it } from 'vitest'
import './op-row'
import type { OpRow } from './op-row'

describe('cchv-op-row', () => {
  it('renders tool, target, a success glyph, and a duration', async () => {
    const el = document.createElement('cchv-op-row') as OpRow
    el.op = {
      id: 'a',
      tool: 'Edit',
      status: 'success',
      started_at: '',
      duration: 200_000_000, // 200ms in ns
      target: 'x.go',
      seq: 1,
    }
    document.body.append(el)
    await el.updateComplete
    const text = el.shadowRoot!.textContent ?? ''
    expect(text).toContain('Edit')
    expect(text).toContain('x.go')
    expect(text).toContain('✔')
    expect(text).toContain('200ms')
    el.remove()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/components/op-row.test.ts`
Expected: FAIL — cannot resolve `./op-row`.

- [ ] **Step 3: Create `web/src/components/op-row.ts`**

```ts
import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import type { Operation } from '../api/types'

const GLYPH: Record<string, string> = {
  running: '▶',
  success: '✔',
  error: '✘',
  neutral: '·',
}

@customElement('cchv-op-row')
export class OpRow extends LitElement {
  @property({ attribute: false }) op!: Operation
  @property({ type: Boolean, reflect: true }) selected = false

  static styles = css`
    :host {
      display: block;
      cursor: pointer;
      padding: 1px 10px 1px 8px;
      border-left: 2px solid transparent;
      white-space: pre;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    :host([selected]) {
      background: var(--sel-bg-foc);
      border-left-color: var(--accent);
    }
    .glyph {
      display: inline-block;
      width: 1ch;
      margin-right: 1ch;
    }
    .running { color: var(--yellow); }
    .success { color: var(--green); }
    .error { color: var(--red); }
    .neutral { color: var(--fg-faint); }
    .tool { color: var(--accent); }
    .target { color: var(--fg); }
    .dur { color: var(--fg-dim); float: right; }
  `

  private dur(ns: number): string {
    if (!ns) return ''
    const ms = ns / 1e6
    return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${Math.round(ms)}ms`
  }

  render() {
    const op = this.op
    return html`<span class="glyph ${op.status}">${GLYPH[op.status] ?? '·'}</span
      ><span class="tool">${op.tool}</span
      ><span class="target"> ${op.target}</span
      ><span class="dur">${this.dur(op.duration)}</span>`
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'cchv-op-row': OpRow
  }
}
```

- [ ] **Step 4: Create `web/src/components/turn-row.ts`**

```ts
import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import type { Turn } from '../api/types'

@customElement('cchv-turn-row')
export class TurnRow extends LitElement {
  @property({ attribute: false }) turn!: Turn

  static styles = css`
    :host {
      display: block;
      padding: 4px 10px;
    }
    .role {
      font-size: 11px;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--magenta);
    }
    .role.user { color: var(--accent); }
    .thinking {
      color: var(--fg-dim);
      font-style: italic;
      white-space: pre-wrap;
    }
    .text {
      color: var(--fg);
      white-space: pre-wrap;
    }
  `

  render() {
    const t = this.turn
    return html`
      <div class="role ${t.role}">${t.role}</div>
      ${t.thinking ? html`<div class="thinking">${t.thinking}</div>` : ''}
      <div class="text">${t.text}</div>
    `
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'cchv-turn-row': TurnRow
  }
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd web && npx vitest run src/components/op-row.test.ts`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/components/op-row.ts web/src/components/turn-row.ts web/src/components/op-row.test.ts
git commit -m "feat(web): op-row + turn-row timeline components"
```

---

### Task 5: inspector subviews (`diff-view`, `code-view`, `raw-view`)

**Files:**
- Create: `web/src/components/diff-view.ts`, `web/src/components/code-view.ts`, `web/src/components/raw-view.ts`
- Test: `web/src/components/diff-view.test.ts`

- [ ] **Step 1: Write the failing test** `web/src/components/diff-view.test.ts`

```ts
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/components/diff-view.test.ts`
Expected: FAIL — cannot resolve `./diff-view`.

- [ ] **Step 3: Create `web/src/components/diff-view.ts`**

```ts
import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import type { DiffOp } from '../api/types'

const SIGN: Record<string, string> = { del: '-', add: '+', context: ' ' }

@customElement('cchv-diff-view')
export class DiffView extends LitElement {
  @property({ attribute: false }) diff: DiffOp[] = []

  static styles = css`
    :host {
      display: block;
      white-space: pre;
      overflow-x: auto;
    }
    .line { padding: 0 10px; }
    .context { color: var(--fg-dim); }
    .del { color: var(--red); background: rgba(248, 81, 73, 0.1); }
    .add { color: var(--green); background: rgba(86, 211, 100, 0.1); }
    .sign { display: inline-block; width: 1ch; }
  `

  render() {
    return html`${this.diff.map(
      (d) =>
        html`<div class="line ${d.kind}"><span class="sign">${SIGN[d.kind] ?? ' '}</span
          >${d.text}</div>`,
    )}`
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'cchv-diff-view': DiffView
  }
}
```

- [ ] **Step 4: Create `web/src/components/code-view.ts`**

```ts
import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'

@customElement('cchv-code-view')
export class CodeView extends LitElement {
  @property() command = ''
  @property() output = ''
  @property({ type: Number }) exitCode?: number

  static styles = css`
    :host { display: block; }
    .cmd { color: var(--accent); padding: 4px 10px; white-space: pre-wrap; }
    .cmd::before { content: '$ '; color: var(--fg-dim); }
    pre { margin: 0; padding: 4px 10px; color: var(--fg); white-space: pre-wrap; }
    .exit { color: var(--fg-dim); padding: 2px 10px; }
  `

  render() {
    return html`
      ${this.command ? html`<div class="cmd">${this.command}</div>` : ''}
      <pre>${this.output}</pre>
      ${this.exitCode != null ? html`<div class="exit">exit ${this.exitCode}</div>` : ''}
    `
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'cchv-code-view': CodeView
  }
}
```

- [ ] **Step 5: Create `web/src/components/raw-view.ts`**

```ts
import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'

@customElement('cchv-raw-view')
export class RawView extends LitElement {
  @property({ attribute: false }) value: unknown

  static styles = css`
    pre {
      margin: 0;
      padding: 6px 10px;
      color: var(--fg-dim);
      white-space: pre-wrap;
      word-break: break-word;
    }
  `

  render() {
    return html`<pre>${JSON.stringify(this.value, null, 2)}</pre>`
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'cchv-raw-view': RawView
  }
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `cd web && npx vitest run src/components/diff-view.test.ts`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/diff-view.ts web/src/components/code-view.ts web/src/components/raw-view.ts web/src/components/diff-view.test.ts
git commit -m "feat(web): inspector subviews (diff/code/raw)"
```

---

### Task 6: container components (`inspector`, `session-list`, `timeline`, `top-bar`)

**Files:**
- Create: `web/src/components/inspector.ts`, `web/src/components/session-list.ts`, `web/src/components/timeline.ts`, `web/src/components/top-bar.ts`
- Test: none new (these are wired-and-smoked via Task 8's `app.test.ts`); type-check is the gate here.

- [ ] **Step 1: Create `web/src/components/inspector.ts`**

```ts
import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import type { OperationDetail } from '../api/types'
import './diff-view'
import './code-view'
import './raw-view'

@customElement('cchv-inspector')
export class Inspector extends LitElement {
  @property({ attribute: false }) detail?: OperationDetail

  static styles = css`
    :host { display: block; height: 100%; overflow: auto; }
    .empty { color: var(--fg-faint); padding: 12px; }
    .hdr {
      color: var(--fg-dim);
      padding: 6px 10px;
      border-bottom: 1px solid var(--border);
    }
    .section {
      color: var(--fg-faint);
      font-size: 11px;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      padding: 8px 10px 2px;
    }
  `

  render() {
    const d = this.detail
    if (!d) return html`<div class="empty">Select an operation</div>`
    return html`
      <div class="hdr">${d.tool}${d.file_path ? ` · ${d.file_path}` : ''}</div>
      ${d.detail_kind === 'diff'
        ? html`<cchv-diff-view .diff=${d.diff ?? []}></cchv-diff-view>`
        : ''}
      ${d.detail_kind === 'output'
        ? html`<cchv-code-view
            .command=${d.command ?? ''}
            .output=${d.output ?? ''}
            .exitCode=${d.exit_code}
          ></cchv-code-view>`
        : ''}
      <div class="section">raw</div>
      <cchv-raw-view .value=${{ pre: d.raw_pre, post: d.raw_post }}></cchv-raw-view>
    `
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'cchv-inspector': Inspector
  }
}
```

- [ ] **Step 2: Create `web/src/components/session-list.ts`**

```ts
import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import type { SessionInfo } from '../api/types'

@customElement('cchv-session-list')
export class SessionList extends LitElement {
  @property({ attribute: false }) sessions: SessionInfo[] = []
  @property() selectedId = ''

  static styles = css`
    :host { display: block; height: 100%; overflow: auto; }
    .row {
      padding: 3px 10px;
      cursor: pointer;
      border-left: 2px solid transparent;
    }
    .row.sel {
      background: var(--sel-bg-foc);
      border-left-color: var(--accent);
    }
    .proj { color: var(--fg); }
    .meta { color: var(--fg-dim); font-size: 11px; }
  `

  private pick(id: string) {
    this.dispatchEvent(
      new CustomEvent('select-session', { detail: id, bubbles: true, composed: true }),
    )
  }

  private project(s: SessionInfo): string {
    if (s.title) return s.title
    if (s.cwd) return s.cwd.split('/').pop() || s.cwd
    return s.id.slice(0, 8)
  }

  render() {
    return html`${this.sessions.map(
      (s) => html`<div
        class="row ${s.id === this.selectedId ? 'sel' : ''}"
        @click=${() => this.pick(s.id)}
      >
        <div class="proj">${this.project(s)}</div>
        <div class="meta">${s.event_count} ev · ${s.id.slice(0, 8)}</div>
      </div>`,
    )}`
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'cchv-session-list': SessionList
  }
}
```

- [ ] **Step 3: Create `web/src/components/timeline.ts`**

```ts
import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import type { TimelineItem } from '../api/types'
import './op-row'
import './turn-row'

@customElement('cchv-timeline')
export class Timeline extends LitElement {
  @property({ attribute: false }) items: TimelineItem[] = []
  @property() selectedOpId = ''

  static styles = css`
    :host { display: block; height: 100%; overflow: auto; }
    .empty { color: var(--fg-faint); padding: 12px; }
  `

  private pickOp(id: string) {
    this.dispatchEvent(
      new CustomEvent('select-op', { detail: id, bubbles: true, composed: true }),
    )
  }

  render() {
    if (!this.items.length) return html`<div class="empty">No activity yet</div>`
    return html`${this.items.map((it) => {
      if (it.kind === 'operation' && it.op) {
        const op = it.op
        return html`<cchv-op-row
          .op=${op}
          ?selected=${op.id === this.selectedOpId}
          @click=${() => this.pickOp(op.id)}
        ></cchv-op-row>`
      }
      if (it.turn) return html`<cchv-turn-row .turn=${it.turn}></cchv-turn-row>`
      return ''
    })}`
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'cchv-timeline': Timeline
  }
}
```

- [ ] **Step 4: Create `web/src/components/top-bar.ts`**

```ts
import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'

@customElement('cchv-top-bar')
export class TopBar extends LitElement {
  @property({ type: Boolean }) live = false
  @property({ type: Boolean }) daemonOk = false

  static styles = css`
    :host {
      display: flex;
      align-items: center;
      gap: 12px;
      height: 28px;
      padding: 0 12px;
      background: var(--bar-bg);
      border-bottom: 1px solid var(--border);
      color: var(--fg-dim);
      font-size: 11px;
    }
    .name { color: var(--fg); font-weight: 600; }
    .ok { color: var(--green); }
    .bad { color: var(--red); }
    .live { color: var(--green); }
  `

  render() {
    return html`
      <span class="name">cchv</span>
      <span class="${this.daemonOk ? 'ok' : 'bad'}"
        >${this.daemonOk ? '● daemon' : '○ daemon'}</span
      >
      ${this.live ? html`<span class="live">live</span>` : ''}
    `
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'cchv-top-bar': TopBar
  }
}
```

- [ ] **Step 5: Type-check the new components**

Run: `cd web && npx tsc --noEmit`
Expected: clean (no errors).

- [ ] **Step 6: Commit**

```bash
git add web/src/components/inspector.ts web/src/components/session-list.ts web/src/components/timeline.ts web/src/components/top-bar.ts
git commit -m "feat(web): inspector, session-list, timeline, top-bar containers"
```

---

### Task 7: `cchv-app` controller + bootstrap

**Files:**
- Create: `web/src/components/app.ts`
- Test: `web/src/components/app.test.ts`
- Modify: `web/src/main.ts` (replace the Task 1 placeholder)

- [ ] **Step 1: Write the failing test** `web/src/components/app.test.ts`

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'
import './app'
import type { App } from './app'

describe('cchv-app', () => {
  beforeEach(() => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('[]', { status: 200 }))
    // happy-dom has no EventSource; provide a no-op stub.
    if (!('EventSource' in globalThis)) {
      // @ts-expect-error minimal test stub
      globalThis.EventSource = class {
        onmessage: ((e: MessageEvent) => void) | null = null
        close() {}
      }
    }
  })

  it('renders the three panes', async () => {
    const el = document.createElement('cchv-app') as App
    document.body.append(el)
    await el.updateComplete
    const root = el.shadowRoot!
    expect(root.querySelector('cchv-session-list')).toBeTruthy()
    expect(root.querySelector('cchv-timeline')).toBeTruthy()
    expect(root.querySelector('cchv-inspector')).toBeTruthy()
    el.remove()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/components/app.test.ts`
Expected: FAIL — cannot resolve `./app`.

- [ ] **Step 3: Create `web/src/components/app.ts`**

```ts
import { css, html, LitElement } from 'lit'
import { customElement, state } from 'lit/decorators.js'
import { api } from '../api/client'
import { StreamController } from '../api/stream'
import type { OperationDetail, SessionInfo, TimelineItem } from '../api/types'
import './top-bar'
import './session-list'
import './timeline'
import './inspector'

@customElement('cchv-app')
export class App extends LitElement {
  @state() private sessions: SessionInfo[] = []
  @state() private selectedSessionId = ''
  @state() private items: TimelineItem[] = []
  @state() private selectedOpId = ''
  @state() private detail?: OperationDetail
  @state() private daemonOk = false
  @state() private live = false

  private stream = new StreamController(() => void this.refreshTimeline())

  static styles = css`
    :host {
      display: grid;
      grid-template-rows: 28px 1fr;
      height: 100vh;
    }
    .panes {
      display: grid;
      grid-template-columns: 240px 1fr 380px;
      min-height: 0;
    }
    .pane {
      min-height: 0;
      overflow: auto;
      border-right: 1px solid var(--border);
      display: flex;
      flex-direction: column;
    }
    .pane:last-child { border-right: none; }
    .ptitle {
      color: var(--fg-dim);
      font-size: 11px;
      letter-spacing: 0.06em;
      text-transform: uppercase;
      padding: 5px 10px;
      border-bottom: 1px solid var(--border);
    }
    .body { flex: 1; min-height: 0; overflow: auto; }
  `

  connectedCallback() {
    super.connectedCallback()
    void this.loadSessions()
    this.addEventListener('select-session', (e: Event) =>
      void this.selectSession((e as CustomEvent<string>).detail),
    )
    this.addEventListener('select-op', (e: Event) =>
      void this.selectOp((e as CustomEvent<string>).detail),
    )
  }

  disconnectedCallback() {
    super.disconnectedCallback()
    this.stream.disconnect()
  }

  private async loadSessions() {
    try {
      this.sessions = await api.sessions()
      this.daemonOk = true
    } catch {
      this.daemonOk = false
    }
  }

  private async selectSession(id: string) {
    this.selectedSessionId = id
    this.selectedOpId = ''
    this.detail = undefined
    await this.refreshTimeline()
    this.stream.connect(id)
    this.live = true
  }

  private async refreshTimeline() {
    if (!this.selectedSessionId) return
    try {
      this.items = await api.timeline(this.selectedSessionId)
      this.daemonOk = true
    } catch {
      this.daemonOk = false
    }
  }

  private async selectOp(opId: string) {
    this.selectedOpId = opId
    try {
      this.detail = await api.operation(this.selectedSessionId, opId)
    } catch {
      this.detail = undefined
    }
  }

  render() {
    return html`
      <cchv-top-bar .live=${this.live} .daemonOk=${this.daemonOk}></cchv-top-bar>
      <div class="panes">
        <div class="pane">
          <div class="ptitle">sessions</div>
          <div class="body">
            <cchv-session-list
              .sessions=${this.sessions}
              .selectedId=${this.selectedSessionId}
            ></cchv-session-list>
          </div>
        </div>
        <div class="pane">
          <div class="ptitle">timeline</div>
          <div class="body">
            <cchv-timeline
              .items=${this.items}
              .selectedOpId=${this.selectedOpId}
            ></cchv-timeline>
          </div>
        </div>
        <div class="pane">
          <div class="ptitle">inspector</div>
          <div class="body">
            <cchv-inspector .detail=${this.detail}></cchv-inspector>
          </div>
        </div>
      </div>
    `
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'cchv-app': App
  }
}
```

- [ ] **Step 4: Replace `web/src/main.ts`** (drop the placeholder)

```ts
import './global.css'
import './components/app'
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd web && npx vitest run src/components/app.test.ts`
Expected: PASS.

- [ ] **Step 6: Run the full frontend suite + a production build**

Run: `cd web && npx vitest run && npm run build`
Expected: all vitest files PASS; `tsc --noEmit` clean; `vite build` writes `internal/web/dist/index.html` + `assets/`.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/app.ts web/src/components/app.test.ts web/src/main.ts
git commit -m "feat(web): cchv-app controller + bootstrap (three-pane UI)"
```

---

### Task 8: `internal/web` — embed + SPA handler

**Files:**
- Create: `internal/web/embed.go`, `internal/web/dist/.gitkeep`
- Test: `internal/web/embed_test.go`

`go:embed all:dist` needs `internal/web/dist` to exist with at least one matching file at compile time. The committed `.gitkeep` satisfies that on a fresh checkout (`all:` includes dotfiles); `make build` later fills `dist` with the real bundle. The handler is testable via `NewHandler(fs.FS)` so tests inject an `fstest.MapFS`.

- [ ] **Step 1: Create the placeholder so the package compiles**

Run:
```bash
mkdir -p internal/web/dist && touch internal/web/dist/.gitkeep
```

- [ ] **Step 2: Write the failing test** `internal/web/embed_test.go`

```go
package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAHandler_ServesIndexAtRoot(t *testing.T) {
	files := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><cchv-app></cchv-app>")}}
	rec := httptest.NewRecorder()
	NewHandler(files).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "cchv-app") {
		t.Fatalf("root: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSPAHandler_ServesAsset(t *testing.T) {
	files := fstest.MapFS{
		"index.html":    {Data: []byte("idx")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	rec := httptest.NewRecorder()
	NewHandler(files).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log(1)" {
		t.Fatalf("asset: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSPAHandler_FallsBackToIndex(t *testing.T) {
	files := fstest.MapFS{"index.html": {Data: []byte("idx")}}
	rec := httptest.NewRecorder()
	NewHandler(files).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/some/spa/route", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "idx" {
		t.Fatalf("fallback: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSPAHandler_UnbuiltNotice(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "make build") {
		t.Fatalf("notice: code=%d body=%q", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/web/ -v`
Expected: FAIL — `undefined: NewHandler` (and no `embed.go`).

- [ ] **Step 4: Create `internal/web/embed.go`**

```go
// Package web embeds the built Lit single-page app (internal/web/dist) and
// serves it as an http.Handler with SPA fallback. The bundle is embedded at
// compile time; a fresh checkout that has not run `make build` embeds only a
// placeholder, and the handler serves a "run make build" notice instead of the
// app. The embed path is internal/web/dist (not the top-level web/dist) because
// go:embed patterns cannot traverse upward out of the source file's directory.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns the production handler serving the embedded dist tree.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// Unreachable: dist is embedded at compile time. Serve the notice.
		return NewHandler(fstest_empty{})
	}
	return NewHandler(sub)
}

// NewHandler builds an SPA handler over files. A request that names an existing
// file is served directly; anything else falls back to index.html. When
// index.html is absent (unbuilt checkout), a plain-text notice is served.
func NewHandler(files fs.FS) http.Handler {
	return &spaHandler{files: files}
}

type spaHandler struct{ files fs.FS }

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	if h.serveFile(w, r, name) {
		return
	}
	// SPA fallback: serve index.html for unknown (client-routed) paths.
	if h.serveFile(w, r, "index.html") {
		return
	}
	// Unbuilt checkout: helpful notice instead of a bare 404.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("cchv web bundle not built. Run `make build` to build and embed the frontend.\n"))
}

// serveFile serves name from the FS if it exists and is a regular file,
// returning true when it handled the response.
func (h *spaHandler) serveFile(w http.ResponseWriter, r *http.Request, name string) bool {
	f, err := h.files.Open(name)
	if err != nil {
		return false
	}
	st, serr := f.Stat()
	_ = f.Close()
	if serr != nil || st.IsDir() {
		return false
	}
	http.ServeFileFS(w, r, h.files, name)
	return true
}

// fstest_empty is a zero-file FS used only on the unreachable fs.Sub error path.
type fstest_empty struct{}

func (fstest_empty) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/web/ -v`
Expected: PASS (4 tests).

- [ ] **Step 6: Commit** (force-add `.gitkeep` past the gitignore)

```bash
git add -f internal/web/dist/.gitkeep
git add internal/web/embed.go internal/web/embed_test.go
git commit -m "feat(web): go:embed SPA handler with build fallback notice"
```

---

### Task 9: daemon — mount the web SPA at `/`

**Files:**
- Modify: `internal/daemon/daemon.go` (import `web`; register `/` in `NewServer`)
- Test: `internal/daemon/api_test.go` (add a root-serves test)

Go's `ServeMux` resolves the most specific matching pattern, so `/api/sessions/`, `/stream`, `/events`, `/healthz`, `/sessions`, `/sessions/` all keep priority over the catch-all `/`. Mounting `/` does not shadow them.

- [ ] **Step 1: Write the failing test** (append to `internal/daemon/api_test.go`)

```go
func TestRootServesWebPage(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("root status %d (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestAPIStillRoutesUnderRootMount(t *testing.T) {
	srv := newTestServer(t, &event.Event{SessionID: "s1", HookEvent: "SessionStart", Raw: []byte(`{}`)})
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/sessions status %d — root mount shadowed the API", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/daemon/ -run 'TestRootServesWebPage|TestAPIStillRoutesUnderRootMount' -v`
Expected: `TestRootServesWebPage` FAILs — `/` returns 404 (no handler registered yet). (`TestAPIStillRoutesUnderRootMount` passes already; it guards against regression.)

- [ ] **Step 3a: Add the import** in `internal/daemon/daemon.go`

In the import block, add:
```go
	"jordandavis.dev/cc-harness-visualizer/internal/web"
```

- [ ] **Step 3b: Register the route** in `NewServer`, after the `/api/sessions/` line:

```go
	s.mux.Handle("/", web.Handler())
```

(The block becomes:)
```go
	s.mux.HandleFunc("/api/sessions", s.handleAPISessions)
	s.mux.HandleFunc("/api/sessions/", s.handleAPISession)
	s.mux.Handle("/", web.Handler())
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/daemon/ -run 'TestRootServesWebPage|TestAPIStillRoutesUnderRootMount' -v`
Expected: PASS (both).

- [ ] **Step 5: Run the full daemon suite** (guard against shadowing regressions)

Run: `go test ./internal/daemon/ -v`
Expected: PASS (all pre-existing tests still green).

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/daemon.go internal/daemon/api_test.go
git commit -m "feat(daemon): serve embedded web SPA at /"
```

---

### Task 10: `cchv serve` launcher (`internal/client.EnsureDaemon` + `internal/serve`)

**Files:**
- Modify: `internal/client/client.go` (add `EnsureDaemon`, `daemonHealthy`)
- Test: `internal/client/client_test.go` (add a `daemonHealthy` test — create the file if absent)
- Create: `internal/serve/serve.go`, `internal/serve/serve_test.go`
- Modify: `cmd/cchv/main.go` (dispatch `serve`)

- [ ] **Step 1: Write the failing test for `daemonHealthy`** (append to `internal/client/client_test.go`; create the file with this content if it does not exist)

```go
package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDaemonHealthy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	if !daemonHealthy(addr) {
		t.Fatalf("daemonHealthy(%q) = false, want true", addr)
	}
	if daemonHealthy("127.0.0.1:1") {
		t.Fatal("daemonHealthy on a dead port = true, want false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/client/ -run TestDaemonHealthy -v`
Expected: FAIL — `undefined: daemonHealthy`.

- [ ] **Step 3: Add `EnsureDaemon` + `daemonHealthy`** to `internal/client/client.go` (append at the end of the file; `time`, `net/http`, and `fmt` are already imported)

```go
// EnsureDaemon returns the daemon's "host:port", spawning the daemon if it is
// not already reachable. It blocks up to ~3s for a freshly-spawned daemon to
// write its port file and answer /healthz. Used by `cchv serve`.
func EnsureDaemon() (string, error) {
	addr := resolveAddr()
	if daemonHealthy(addr) {
		return addr, nil
	}
	spawnDaemon()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		addr = resolveAddr() // a fresh daemon may pick a new port
		if daemonHealthy(addr) {
			return addr, nil
		}
	}
	return "", fmt.Errorf("daemon did not become healthy at %s", addr)
}

// daemonHealthy reports whether GET http://addr/healthz returns 200 promptly.
func daemonHealthy(addr string) bool {
	c := &http.Client{Timeout: 200 * time.Millisecond}
	resp, err := c.Get("http://" + addr + "/healthz")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/client/ -run TestDaemonHealthy -v`
Expected: PASS.

- [ ] **Step 5: Write the failing test** `internal/serve/serve_test.go`

```go
package serve

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRun_OpensResolvedURL(t *testing.T) {
	var opened string
	code := run(
		func() (string, error) { return "127.0.0.1:7842", nil },
		func(u string) error { opened = u; return nil },
		&bytes.Buffer{},
	)
	if code != 0 {
		t.Fatalf("code=%d, want 0", code)
	}
	if opened != "http://127.0.0.1:7842/" {
		t.Fatalf("opened=%q, want http://127.0.0.1:7842/", opened)
	}
}

func TestRun_EnsureFailureReturns1(t *testing.T) {
	code := run(
		func() (string, error) { return "", errors.New("nope") },
		func(string) error { return nil },
		&bytes.Buffer{},
	)
	if code != 1 {
		t.Fatalf("code=%d, want 1", code)
	}
}

func TestRun_OpenFailureStillSucceeds(t *testing.T) {
	out := &bytes.Buffer{}
	code := run(
		func() (string, error) { return "127.0.0.1:9000", nil },
		func(string) error { return errors.New("no browser") },
		out,
	)
	if code != 0 {
		t.Fatalf("code=%d, want 0 (open failure is non-fatal)", code)
	}
	if !strings.Contains(out.String(), "127.0.0.1:9000") {
		t.Fatalf("out=%q, want it to print the URL for manual opening", out.String())
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./internal/serve/ -v`
Expected: FAIL — no such package / `undefined: run`.

- [ ] **Step 7: Create `internal/serve/serve.go`**

```go
// Package serve implements `cchv serve`: ensure the daemon is running, then
// open the system browser at the daemon's embedded web UI. It is a thin
// launcher, not a second server — the daemon already serves the UI at /.
package serve

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"

	"jordandavis.dev/cc-harness-visualizer/internal/client"
)

// Run is the CLI entrypoint for `cchv serve`. args is currently unused.
func Run(args []string) int {
	_ = args
	return run(client.EnsureDaemon, openBrowser, os.Stdout)
}

// run is the testable core. ensure returns the daemon "host:port"; open is
// invoked with the UI URL. An open failure is non-fatal — the URL is printed
// so the user can open it manually.
func run(ensure func() (string, error), open func(string) error, out io.Writer) int {
	addr, err := ensure()
	if err != nil {
		fmt.Fprintln(os.Stderr, "serve: "+err.Error())
		return 1
	}
	url := "http://" + addr + "/"
	fmt.Fprintf(out, "cchv serve: %s\n", url)
	if err := open(url); err != nil {
		fmt.Fprintln(os.Stderr, "serve: open browser: "+err.Error())
	}
	return 0
}

// openBrowser opens url in the system default browser.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `go test ./internal/serve/ -v`
Expected: PASS (3 tests).

- [ ] **Step 9: Dispatch `serve`** in `cmd/cchv/main.go`

Add the import:
```go
	"jordandavis.dev/cc-harness-visualizer/internal/serve"
```

Add a case to the `switch` (after the `tui` case):
```go
	case "serve":
		os.Exit(serve.Run(rest))
```

Add a usage line in `usage()` after the `tui` line:
```go
  cchv serve      ensure the daemon is up and open the web UI in a browser
```

- [ ] **Step 10: Build + full Go test**

Run: `go build ./... && go test ./...`
Expected: build OK; all Go packages PASS.

- [ ] **Step 11: Commit**

```bash
git add internal/client/client.go internal/client/client_test.go internal/serve/serve.go internal/serve/serve_test.go cmd/cchv/main.go
git commit -m "feat(serve): cchv serve launcher + client.EnsureDaemon"
```

---

### Task 11: Makefile + build wiring

**Files:**
- Create: `Makefile`

The Makefile is the source of truth for building with the embedded frontend, so `go build` never embeds a stale `dist`. `make build` runs the npm build (into `internal/web/dist`) first, then `go build`.

- [ ] **Step 1: Create `Makefile`** (use TAB indentation for recipe lines)

```makefile
BINARY := cchv

.PHONY: build web go-build test test-web clean

# Full build: frontend first (populates internal/web/dist), then the embedded binary.
build: web go-build

web:
	cd web && npm ci && npm run build

go-build:
	go build -o $(BINARY) ./cmd/cchv

# Go tests only (fast; no Node required).
test:
	go test ./...

# Frontend unit tests.
test-web:
	cd web && npm ci && npm run test

clean:
	rm -f $(BINARY)
	rm -rf internal/web/dist
	mkdir -p internal/web/dist
	touch internal/web/dist/.gitkeep
```

- [ ] **Step 2: Verify the full build**

Run: `make build`
Expected: npm build emits `internal/web/dist/{index.html,assets/}`; `go build` produces `./cchv`. No errors.

- [ ] **Step 3: Verify Go tests still pass via the Makefile**

Run: `make test`
Expected: all Go packages PASS.

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "build: Makefile with web+go build and test targets"
```

---

### Task 12: end-to-end smoke against the built binary

**Files:** none (manual verification + a status note).

- [ ] **Step 1: Build the embedded binary**

Run: `make build`
Expected: `./cchv` exists and embeds the real bundle (not just the notice).

- [ ] **Step 2: Confirm the binary serves the SPA, not the notice**

Run:
```bash
./cchv daemon --port 7843 &
sleep 1
curl -s localhost:7843/ | head -c 300; echo
curl -s -o /dev/null -w '%{http_code}\n' localhost:7843/api/sessions
```
Expected: `/` returns HTML containing `<cchv-app>` (or the Vite-injected module script), NOT the "make build" notice; `/api/sessions` returns `200`.

- [ ] **Step 3: Post a paired Edit and confirm the timeline + detail endpoints**

Run:
```bash
curl -s -XPOST localhost:7843/events -d '{"session_id":"smoke","hook_event":"PreToolUse","tool_name":"Edit","raw":{"tool_use_id":"z","tool_input":{"file_path":"x.go","old_string":"a","new_string":"b"}}}'
curl -s -XPOST localhost:7843/events -d '{"session_id":"smoke","hook_event":"PostToolUse","tool_name":"Edit","raw":{"tool_use_id":"z","tool_response":{"exit_code":0}}}'
sleep 1
curl -s localhost:7843/api/sessions/smoke/timeline | head -c 400; echo
curl -s localhost:7843/api/sessions/smoke/operations/z | head -c 400; echo
```
Expected: timeline shows one `"kind":"operation"` with `"status":"success"`; the operation detail shows `"detail_kind":"diff"` with a `diff` array. (This re-confirms Plan 1's API through the embedded binary.)

- [ ] **Step 4: Stop the daemon**

Run: `kill %1`

- [ ] **Step 5 (optional): exercise `cchv serve`**

Run: `./cchv serve` (it ensures a daemon, prints the URL, and tries to open a browser; in a headless env the open may fail — that is non-fatal and the URL still prints). Stop any daemon it spawned with the pid in `daemon.pid` afterwards.

- [ ] **Step 6: Record completion**

Add a `## Status` line at the top of this plan recording that Plan 2 is complete and smoke-verified, then:
```bash
git commit -am "docs: Plan 2 (web UI frontend) complete and smoke-verified"
```

---

## Self-review checklist (run after implementing all tasks)

- [ ] `go build ./...` and `go test ./...` are fully green (model, source, daemon, client, serve, web, tui).
- [ ] `cd web && npm run build` type-checks clean and emits `internal/web/dist/{index.html,assets}`.
- [ ] `cd web && npm run test` is green (client, stream, op-row, diff-view, app).
- [ ] `make build` produces a single `./cchv` that serves the real SPA at `/` (not the unbuilt notice).
- [ ] `/api/sessions`, `/stream`, `/events`, `/healthz`, `/sessions`, `/sessions/` all still route correctly under the `/` catch-all mount.
- [ ] No TS type referenced in a component is undefined in `api/types.ts`; wire field names match Plan 1's Go JSON tags (snake_case; `duration` in ns).
- [ ] `internal/web/dist/.gitkeep` is committed; `internal/web/dist/*` and `web/node_modules` are gitignored.
- [ ] The TUI still works unchanged (`cchv tui` against a running daemon) — Plan 2 touches no TUI code.

## What this plan deliberately defers (fast-follows, NOT built here)

- Keyboard navigation + global keyboard controller; filter (`/`), error-hop, folding, yank/copy.
- List virtualization, syntax-highlighting grammars (Shiki), light/adaptive theme, density/reduced-motion toggles.
- Seq-cursor pagination wiring on the client (currently a full-timeline refetch on each nudge) + `Last-Event-ID` catch-up.
- Model B streaming (server-pushed derived deltas).
- CI split (a Node-free Go job + a `make build` release job). No CI exists in-repo yet; add when CI is introduced.
- Consolidating the TUI's own derivation onto `internal/model` (tracked in Plan 1's deferred list).
- Full E2E browser test suite.
