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
