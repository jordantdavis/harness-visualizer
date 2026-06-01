import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import type { DiffOp } from '../api/types'

const SIGN: Record<string, string> = { del: '-', add: '+', context: ' ' }

@customElement('hv-diff-view')
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
    return html`<div class="lines">${this.diff.map(
      (d) =>
        html`<div class="line ${d.kind}"><span class="sign">${SIGN[d.kind] ?? ' '}</span
          >${d.text}</div>`,
    )}</div>`
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'hv-diff-view': DiffView
  }
}
