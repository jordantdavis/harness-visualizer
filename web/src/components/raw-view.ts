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
