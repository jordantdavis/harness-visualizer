import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import type { Operation, Status } from '../api/types'
import { formatClock, formatFull } from '../util/time'

const GLYPH = {
  running: '▶',
  success: '✔',
  error: '✘',
  neutral: '·',
} satisfies Record<Status, string>

/** Single timeline operation row: clock · status glyph + tool name + target + duration. */
@customElement('hv-op-row')
export class OpRow extends LitElement {
  @property({ attribute: false }) op!: Operation
  @property({ type: Boolean, reflect: true }) selected = false

  static styles = css`
    :host {
      display: block;
      cursor: pointer;
      padding: 1px 10px 1px 8px;
      border-left: 2px solid transparent;
      white-space: pre;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    :host([selected]) {
      background: var(--sel-bg-foc);
      border-left-color: var(--accent);
    }
    .time {
      display: inline-block;
      width: 8ch;
      color: var(--fg-faint);
      margin-right: 1ch;
      cursor: default;
    }
    .glyph {
      display: inline-block;
      width: 1ch;
      margin-right: 1ch;
    }
    .running { color: var(--yellow); }
    .success { color: var(--green); }
    .error { color: var(--red); }
    .neutral { color: var(--fg-faint); }
    .tool { color: var(--accent); }
    .target { color: var(--fg); }
    .dur { color: var(--fg-dim); float: right; }
  `

  /** Format nanoseconds as human-readable duration string (e.g. "<1ms", "200ms", "1.2s"). */
  private dur(ns: number): string {
    if (ns <= 0) return ''
    const ms = ns / 1e6
    if (ms < 1) return '<1ms'
    return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${Math.round(ms)}ms`
  }

  render() {
    const op = this.op
    const clock = formatClock(op.started_at)
    return html`<span class="time" title=${formatFull(op.started_at)}>${clock}</span
      ><span class="glyph ${op.status}">${GLYPH[op.status] ?? '·'}</span
      ><span class="tool">${op.tool}</span
      ><span class="target"> ${op.target}</span
      ><span class="dur">${this.dur(op.duration)}</span>`
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'hv-op-row': OpRow
  }
}
