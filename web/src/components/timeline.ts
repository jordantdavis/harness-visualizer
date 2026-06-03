import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import type { TimelineItem } from '../api/types'
import './op-row'
import './turn-row'
import './lane-event-row'

@customElement('hv-timeline')
export class Timeline extends LitElement {
  @property({ attribute: false }) items: TimelineItem[] = []
  @property() selectedOpId = ''

  static styles = css`
    /* The enclosing column .body owns scroll; this host just flows. */
    :host { display: block; }
    .empty { color: var(--fg-faint); padding: 12px; }
  `

  private pickOp(id: string) {
    this.dispatchEvent(
      new CustomEvent('select-op', { detail: id, bubbles: true, composed: true }),
    )
  }

  render() {
    if (!this.items.length) return html`<div class="empty">No activity yet</div>`
    return html`<div class="items">
      ${this.items.map((it) => {
        if (it.kind === 'operation' && it.op) {
          const op = it.op
          return html`<hv-op-row
            .op=${op}
            ?selected=${op.id !== '' && op.id === this.selectedOpId}
            @click=${() => op.id && this.pickOp(op.id)}
          ></hv-op-row>`
        }
        if (it.kind === 'turn' && it.turn) return html`<hv-turn-row .turn=${it.turn}></hv-turn-row>`
        if (it.kind === 'event' && it.event) return html`<hv-lane-event-row .event=${it.event}></hv-lane-event-row>`
        return ''
      })}
    </div>`
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'hv-timeline': Timeline
  }
}
