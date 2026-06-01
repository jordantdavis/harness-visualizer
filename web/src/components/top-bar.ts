import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'

@customElement('hv-top-bar')
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
      <span class="name">hv</span>
      <span class="${this.daemonOk ? 'ok' : 'bad'}"
        >${this.daemonOk ? '● daemon' : '○ daemon'}</span
      >
      ${this.live ? html`<span class="live">live</span>` : ''}
    `
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'hv-top-bar': TopBar
  }
}
