import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import type { LaneEvent } from '../api/types'
import { lookupHook } from '../api/hooks'

/** Single timeline lane-event row: glyph + label + gist. Glyph/label come
 *  from the shared hook registry (fetched by api/hooks.ts) so the TUI and
 *  web client cannot drift on rendering. */
@customElement('hv-lane-event-row')
export class LaneEventRow extends LitElement {
  @property({ attribute: false }) event!: LaneEvent

  static styles = css`
    :host { display: block; }
    .row {
      padding: 1px 10px 1px 8px;
      white-space: pre;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .glyph { display: inline-block; width: 2ch; margin-right: 1ch; }
    .label { color: var(--accent); }
    .gist { color: var(--fg); margin-left: 1ch; }
    .sev-dim { color: var(--fg-faint); }
    .sev-dim .label, .sev-dim .gist { color: var(--fg-faint); }
    .sev-warn .glyph { color: var(--yellow); }
    .sev-error .glyph, .sev-error .label { color: var(--red); }
  `

  render() {
    const ev = this.event
    const meta = lookupHook(ev.hook_event)
    const gist = ev.gist || ev.hook_event
    const sev = ev.severity || meta.severity || 'info'
    return html`<div class="row sev-${sev}"
      ><span class="glyph">${meta.glyph}</span
      ><span class="label">${meta.label}</span
      ><span class="gist">${gist}</span
    ></div>`
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'hv-lane-event-row': LaneEventRow
  }
}
