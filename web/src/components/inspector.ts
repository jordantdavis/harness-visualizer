import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import type { OperationDetail } from '../api/types'
import { formatFull } from '../util/time'
import './diff-view'
import './code-view'
import './raw-view'

@customElement('hv-inspector')
export class Inspector extends LitElement {
  @property({ attribute: false }) detail?: OperationDetail
  /** ISO timestamp of the selected item; shown as full datetime in the header. */
  @property() selectedAt?: string

  static styles = css`
    /* The enclosing column .body owns scroll; this host just flows. */
    :host { display: block; }
    .empty { color: var(--fg-faint); padding: 12px; }
    .hdr {
      color: var(--fg-dim);
      padding: 6px 10px;
      border-bottom: 1px solid var(--border);
    }
    .hdr-time {
      color: var(--fg-faint);
      font-size: 11px;
      margin-top: 2px;
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
    const ts = formatFull(this.selectedAt ?? '')
    return html`
      <div class="hdr">
        <div>${d.tool}${d.file_path ? ` · ${d.file_path}` : ''}</div>
        ${ts ? html`<div class="hdr-time">${ts}</div>` : ''}
      </div>
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
