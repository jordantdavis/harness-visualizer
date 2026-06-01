import { css, html, LitElement, nothing } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import type { Turn } from '../api/types'
import { formatClock, formatFull } from '../util/time'

/** Single conversation turn row: clock · role label + optional thinking block + text. */
@customElement('hv-turn-row')
export class TurnRow extends LitElement {
  @property({ attribute: false }) turn!: Turn

  static styles = css`
    :host {
      display: block;
      padding: 4px 10px;
    }
    .header {
      display: flex;
      align-items: baseline;
      gap: 1ch;
    }
    .time {
      width: 8ch;
      color: var(--fg-faint);
      font-size: 11px;
      flex-shrink: 0;
      cursor: default;
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
      <div class="header">
        <span class="time" title=${formatFull(t.at)}>${formatClock(t.at)}</span>
        <span class="role ${t.role}">${t.role}</span>
      </div>
      ${t.thinking ? html`<div class="thinking">${t.thinking}</div>` : nothing}
      <div class="text">${t.text}</div>
    `
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'hv-turn-row': TurnRow
  }
}
