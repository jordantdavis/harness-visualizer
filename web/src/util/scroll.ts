/**
 * Whether a scroll container is at (or within `slack` px of) its bottom.
 *
 * Used to decide live-stream autoscroll: when the Timeline is pinned to the
 * tail, new events keep it pinned; when the user has scrolled up, their
 * position is preserved. An empty/unscrollable container (0/0/0) counts as
 * pinned so the first events stick to the tail rather than reading as
 * "scrolled up".
 */
export function isPinnedToBottom(
  scrollTop: number,
  scrollHeight: number,
  clientHeight: number,
  slack = 32,
): boolean {
  return scrollHeight - clientHeight - scrollTop <= slack
}
