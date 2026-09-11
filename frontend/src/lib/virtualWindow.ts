export type VirtualWindow = {
  start: number;
  end: number;
  totalHeight: number;
  paddingTop: number;
  paddingBottom: number;
};

/**
 * Calculate the bounded render window used by VirtualList.
 * Keeping this arithmetic pure makes large-fixture coverage deterministic
 * without requiring a browser or a timing-sensitive render benchmark.
 */
export function getVirtualWindow(
  itemCount: number,
  itemHeight: number,
  viewportHeight: number,
  scrollTop: number,
  overscan = 6,
): VirtualWindow {
  const count = Math.max(0, Math.floor(itemCount));
  const height = Math.max(1, itemHeight);
  const viewport = Math.max(height, viewportHeight);
  const top = Math.max(0, scrollTop);
  const totalHeight = count * height;
  const start = Math.max(0, Math.floor(top / height) - Math.max(0, overscan));
  const end = Math.min(count, Math.ceil((top + viewport) / height) + Math.max(0, overscan));
  const visibleHeight = Math.max(0, end - start) * height;
  const paddingTop = start * height;
  const paddingBottom = Math.max(0, totalHeight - paddingTop - visibleHeight);
  return { start, end, totalHeight, paddingTop, paddingBottom };
}
