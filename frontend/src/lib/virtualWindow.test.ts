import assert from "node:assert/strict";
import { getVirtualWindow } from "./virtualWindow";

const large = getVirtualWindow(100_000, 72, 720, 50_000 * 72);
assert.equal(large.totalHeight, 7_200_000);
assert.equal(large.start, 49_994);
assert.equal(large.end, 50_016);
assert.ok(large.end - large.start <= 32, "large fixtures must render only a bounded window");
assert.equal(large.paddingTop, large.start * 72);
assert.equal(large.paddingBottom, large.totalHeight - large.paddingTop - (large.end - large.start) * 72);

const first = getVirtualWindow(3, 72, 720, 0);
assert.deepEqual(first, {
  start: 0,
  end: 3,
  totalHeight: 216,
  paddingTop: 0,
  paddingBottom: 0,
});

console.log("virtual window tests passed (large fixture stays bounded)");
