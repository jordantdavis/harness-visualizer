import { css, html, LitElement } from 'lit'
import { customElement, state } from 'lit/decorators.js'
import { api } from '../api/client'
import { loadHooks } from '../api/hooks'
import { StreamController } from '../api/stream'
import type { OperationDetail, SessionInfo, TimelineItem } from '../api/types'
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

  static styles = css`
    :host {
      display: grid;
      grid-template-rows: 28px 1fr;
      height: 100vh;
    }
    .panes {
      display: grid;
      grid-template-columns: 240px 1fr 520px;
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

  constructor() {
    super()
    this.addEventListener('select-session', (e: Event) => {
      void this.selectSession((e as CustomEvent<string>).detail)
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
    await this.refreshTimeline()
    this.stream.connect(id)
    this.live = true
  }

  private async refreshTimeline() {
    const id = this.selectedSessionId
    if (!id) return
    try {
      const items = await api.timeline(id)
      if (this.selectedSessionId !== id) return // a newer selection superseded this fetch
      this.items = items
      this.daemonOk = true
    } catch {
      if (this.selectedSessionId !== id) return
      this.daemonOk = false
      this.live = false
    }
  }

  private async selectOp(opId: string) {
    if (!this.selectedSessionId) return
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
          <div class="body">
            <hv-session-list
              .sessions=${this.sessions}
              .selectedId=${this.selectedSessionId}
              .now=${this.now}
            ></hv-session-list>
          </div>
        </div>
        <div class="pane">
          <div class="ptitle">timeline</div>
          <div class="body">
            <hv-timeline
              .items=${this.items}
              .selectedOpId=${this.selectedOpId}
            ></hv-timeline>
          </div>
        </div>
        <div class="pane">
          <div class="ptitle">inspector</div>
          <div class="body">
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
