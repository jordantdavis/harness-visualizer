import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import type { SessionInfo } from '../api/types'
import { formatFull, formatRelative } from '../util/time'

@customElement('hv-session-list')
export class SessionList extends LitElement {
  @property({ attribute: false }) sessions: SessionInfo[] = []
  @property() selectedId = ''
  /** Current time for relative-time labels. Updated by <hv-app> every 30s. */
  @property({ attribute: false }) now: Date = new Date()

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
    .age { cursor: default; }
  `

  private pick(id: string) {
    this.dispatchEvent(
      new CustomEvent('select-session', { detail: id, bubbles: true, composed: true }),
    )
  }

  private project(s: SessionInfo): string {
    if (s.title) return s.title
    if (s.cwd) return s.cwd.replace(/\/+$/, '').split('/').pop() || s.cwd
    return s.id.slice(0, 8)
  }

  /**
   * Timestamp shown as the row's age — the server's sort key (last activity),
   * falling back to mod_time when no event carried a timestamp. Mirrors the
   * daemon's effectiveActivity ordering so the label matches the list order.
   */
  private activity(s: SessionInfo): string {
    return s.last_activity || s.mod_time
  }

  render() {
    return html`<div class="list">
      ${this.sessions.map(
        (s) => html`<div
          class="row ${s.id === this.selectedId ? 'sel' : ''}"
          @click=${() => this.pick(s.id)}
        >
          <div class="proj">${this.project(s)}</div>
          <div class="meta">
            ${s.event_count} ev · ${s.id.slice(0, 8)} ·
            <span class="age" title=${formatFull(this.activity(s))}
              >${formatRelative(this.activity(s), this.now)}</span
            >
          </div>
        </div>`,
      )}
    </div>`
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'hv-session-list': SessionList
  }
}
