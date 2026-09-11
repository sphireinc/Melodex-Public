import assert from "node:assert/strict";
import { toolReadinessDetail, toolReadinessLabel } from "./toolReadiness";

type TestCase = { name: string; run: () => void };

const tests: TestCase[] = [
  {
    name: "labels cached probe results without changing the settings vocabulary",
    run: () => {
      assert.equal(toolReadinessLabel({ tool: "yt-dlp", status: "ready" }, true), "Ready");
      assert.equal(toolReadinessLabel({ tool: "yt-dlp", status: "missing" }, false), "Missing");
      assert.equal(toolReadinessLabel({ tool: "yt-dlp", status: "probe-failed" }, true), "Check failed");
      assert.equal(toolReadinessLabel({ tool: "yt-dlp", status: "invalid-version" }, true), "Invalid version");
      assert.equal(toolReadinessLabel({ tool: "yt-dlp", status: "timeout" }, true), "Timed out");
      assert.equal(toolReadinessLabel({ tool: "yt-dlp", status: "unchecked" }, true), "Not tested");
    },
  },
  {
    name: "combines bounded probe error and remediation for recovery guidance",
    run: () => {
      assert.equal(
        toolReadinessDetail({
          tool: "ffmpeg",
          status: "probe-failed",
          error: "permission denied",
          remediation: "Check the configured ffmpeg path and test it again.",
        }),
        "permission denied Check the configured ffmpeg path and test it again.",
      );
      assert.equal(toolReadinessDetail(undefined), "Run the tool test to verify it before starting an import.");
      assert.equal(
        toolReadinessDetail({
          tool: "yt-dlp",
          status: "invalid-version",
          error: "probe returned invalid version text",
          remediation: "Verify the executable path and test it again.",
        }),
        "probe returned invalid version text Verify the executable path and test it again.",
      );
    },
  },
];

for (const test of tests) {
  test.run();
  console.log(`ok - ${test.name}`);
}
