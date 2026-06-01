import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import type { OperationDetail } from '../api/types'
import './diff-view'
import './code-view'
import './raw-view'

@customElement('hv-inspector')
export class Inspector extends LitElement {
  @property({ attribute: false }) detail?: OperationDetail

  static styles = css`
    :host { display: block; height: 100%; overflow: auto; }
    .empty { color: var(--fg-faint); padding: 12px; }
    .hdr {
      color: var(--fg-dim);
      padding: 6px 10px;
      border-bottom: 1px solid var(--border);
    }
    .section {
      color: var(--fg-faint);
      font-size: 11px;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      padding: 8px 10px 2px;
    }
  `

  render() {
    const d = this.detail
    if (!d) return html`<div class="empty">Select an operation</div>`
    return html`
      <div class="hdr">${d.tool}${d.file_path ? ` · ${d.file_path}` : ''}</div>
      ${d.detail_kind === 'diff'
        ? html`<hv-diff-view .diff=${d.diff ?? []}></hv-diff-view>`
        : ''}
      ${d.detail_kind === 'output'
        ? html`<hv-code-view
            .command=${d.command ?? ''}
            .output=${d.output ?? ''}
            .exitCode=${d.exit_code}
          ></hv-code-view>`
        : ''}
      <div class="section">raw</div>
      <hv-raw-view .value=${{ pre: d.raw_pre, post: d.raw_post }}></hv-raw-view>
    `
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'hv-inspector': Inspector
  }
}
