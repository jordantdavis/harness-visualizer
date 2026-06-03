import { css, html, LitElement } from 'lit'
import { customElement, query, state } from 'lit/decorators.js'
import { api } from '../api/client'
import { loadHooks } from '../api/hooks'
import { StreamController } from '../api/stream'
import type { OperationDetail, SessionInfo, TimelineItem } from '../api/types'
import { isPinnedToBottom } from '../util/scroll'
import './top-bar'
import './session-list'
import './timeline'
import './inspector'

@customElement('hv-app')
export class App extends LitElement {
  @state() private sessions: SessionInfo[] = []
  @state() private selectedSessionId = ''
  @state() private items: TimelineItem[] = []
  @state() private selectedOpId = ''
  @state() private detail?: OperationDetail
  @state() private daemonOk = false
  @state() private live = false
  /** Refreshed every 30s to keep relative-time labels current. */
  @state() private now: Date = new Date()
  /** ISO timestamp of the currently selected timeline item (for inspector header). */
  @state() private selectedAt?: string

  private stream = new StreamController(() => void this.refreshTimeline())
  private nowTimer?: ReturnType<typeof setInterval>

  /** Snap the timeline body back to the top after the next render. */
  private resetTimelineScroll = false
  /** Snap the inspector body back to the top after the next render. */
  private resetInspectorScroll = false
  /** Keep the timeline pinned to the tail after the next render (live append). */
  private stickToTail = false

  @query('.body-timeline') private timelineBody?: HTMLElement
  @query('.body-inspector') private inspectorBody?: HTMLElement

  static styles = css`
    :host {
      display: grid;
      /* minmax(0,…) caps the content row at the viewport — a bare 1fr would
         grow to its tall min-content and push the columns past the fold. */
      grid-template-rows: 28px minmax(0, 1fr);
      height: 100vh;
      /* The shell never scrolls; each column owns its own scroll region. */
      overflow: hidden;
    }
    .panes {
      display: grid;
      grid-template-columns: 240px 1fr 520px;
      /* Pin the single row to the available height; minmax(0,…) lets the panes
         shrink below their content so each .body (not the page) scrolls. */
      grid-template-rows: minmax(0, 1fr);
      min-height: 0;
      /* Bound the grid to the row track so the page body never scrolls. */
      overflow: hidden;
    }
    .pane {
      min-height: 0;
      border-right: 1px solid var(--border);
      display: flex;
      flex-direction: column;
      /* The pane clips; its .body is the single scroll owner. */
      overflow: hidden;
    }
    .pane:last-child { border-right: none; }
    .ptitle {
      /* Pinned column header — stays put while the body scrolls beneath it. */
      flex: 0 0 auto;
      color: var(--fg-dim);
      font-size: 11px;
      letter-spacing: 0.06em;
      text-transform: uppercase;
      padding: 5px 10px;
      border-bottom: 1px solid var(--border);
    }
    .body {
      flex: 1;
      min-height: 0;
      overflow-y: auto;
      /* Reserve the gutter so content doesn't shift when the bar toggles. */
      scrollbar-gutter: stable;
    }
  `

  constructor() {
    super()
    this.addEventListener('select-session', (e: Event) => {
      void this.selectSession((e as CustomEvent<string>).detail)
    })
    this.addEventListener('delete-session', (e: Event) => {
      void this.deleteSession((e as CustomEvent<string>).detail)
    })
    this.addEventListener('select-op', (e: Event) => {
      void this.selectOp((e as CustomEvent<string>).detail)
    })
  }

  connectedCallback() {
    super.connectedCallback()
    void this.loadSessions()
    void loadHooks()
    this.nowTimer = setInterval(() => { this.now = new Date() }, 30_000)
  }

  disconnectedCallback() {
    super.disconnectedCallback()
    this.stream.disconnect()
    clearInterval(this.nowTimer)
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
    this.selectedAt = undefined
    // A session change is a fresh context: snap both columns to the top.
    this.resetTimelineScroll = true
    this.resetInspectorScroll = true
    await this.refreshTimeline()
    this.stream.connect(id)
    this.live = true
  }

  private async deleteSession(id: string) {
    try {
      await api.deleteSession(id)
    } catch {
      this.daemonOk = false
      return
    }
    // Optimistically drop the row; if the deleted session was open, clear the
    // timeline/inspector and stop the live stream.
    this.sessions = this.sessions.filter((s) => s.id !== id)
    if (this.selectedSessionId === id) {
      this.selectedSessionId = ''
      this.items = []
      this.selectedOpId = ''
      this.detail = undefined
      this.selectedAt = undefined
      this.stream.disconnect()
      this.live = false
    }
    // Refetch to stay authoritative with the daemon.
    void this.loadSessions()
  }

  private async refreshTimeline() {
    const id = this.selectedSessionId
    if (!id) return
    try {
      const items = await api.timeline(id)
      if (this.selectedSessionId !== id) return // a newer selection superseded this fetch
      // Decide tail-stick against the still-rendered (old) content, unless a
      // selection reset already owns this render. If the user is pinned to the
      // tail, keep them pinned; if scrolled up, preserve their position.
      if (!this.resetTimelineScroll) {
        const el = this.timelineBody
        this.stickToTail = el
          ? isPinnedToBottom(el.scrollTop, el.scrollHeight, el.clientHeight)
          : false
      }
      this.items = items
      this.daemonOk = true
    } catch {
      if (this.selectedSessionId !== id) return
      this.daemonOk = false
      this.live = false
    }
  }

  /**
   * Apply pending scroll positions after the DOM reflects the new state.
   * Selection resets are instant (direct scrollTop assignment) — these are
   * state jumps, not navigation, so no smooth scrolling.
   */
  protected updated() {
    if (this.resetInspectorScroll) {
      this.resetInspectorScroll = false
      if (this.inspectorBody) this.inspectorBody.scrollTop = 0
    }
    if (this.resetTimelineScroll) {
      this.resetTimelineScroll = false
      this.stickToTail = false
      // Reset-to-top is content-independent, so it's safe synchronously.
      if (this.timelineBody) this.timelineBody.scrollTop = 0
    } else if (this.stickToTail) {
      this.stickToTail = false
      // The child <hv-timeline> renders its new rows in a later update tick,
      // so the new scrollHeight isn't available yet here. Defer the pin a frame
      // so we scroll to the real new tail, not the stale one.
      const apply = () => {
        const el = this.timelineBody
        if (el) el.scrollTop = el.scrollHeight
      }
      if (typeof requestAnimationFrame === 'function') requestAnimationFrame(apply)
      else apply()
    }
  }

  private async selectOp(opId: string) {
    if (!this.selectedSessionId) return
    // Reset the inspector scroll only when the operation actually changes, so a
    // long tool result isn't interrupted mid-read by live re-renders.
    if (opId !== this.selectedOpId) this.resetInspectorScroll = true
    this.selectedOpId = opId

    // Capture the timestamp of the selected item for the inspector header.
    const item = this.items.find(
      (it) => it.kind === 'operation' && it.op?.id === opId,
    )
    this.selectedAt = item?.at

    try {
      this.detail = await api.operation(this.selectedSessionId, opId)
    } catch {
      this.detail = undefined
    }
  }

  render() {
    return html`
      <hv-top-bar .live=${this.live} .daemonOk=${this.daemonOk}></hv-top-bar>
      <div class="panes">
        <div class="pane">
          <div class="ptitle">sessions</div>
          <div class="body body-sessions" role="region" aria-label="Sessions">
            <hv-session-list
              .sessions=${this.sessions}
              .selectedId=${this.selectedSessionId}
              .now=${this.now}
            ></hv-session-list>
          </div>
        </div>
        <div class="pane">
          <div class="ptitle">timeline</div>
          <div class="body body-timeline" role="region" aria-label="Timeline" aria-live="polite">
            <hv-timeline
              .items=${this.items}
              .selectedOpId=${this.selectedOpId}
            ></hv-timeline>
          </div>
        </div>
        <div class="pane">
          <div class="ptitle">inspector</div>
          <div class="body body-inspector" role="region" aria-label="Inspector">
            <hv-inspector
              .detail=${this.detail}
              .selectedAt=${this.selectedAt}
            ></hv-inspector>
          </div>
        </div>
      </div>
    `
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'hv-app': App
  }
}
