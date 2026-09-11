import assert from "node:assert/strict";
import {
  serializeVideoDiagnosticContext,
  videoDiagnosticContext,
  videoSourceType,
} from "./videoDiagnostics";

type TestCase = { name: string; run: () => void };

const tests: TestCase[] = [
  {
    name: "classifies the source without exposing a path or URL",
    run: () => {
      assert.equal(videoSourceType("/library/video.mp4", "", ""), "downloaded-file");
      assert.equal(videoSourceType("", "wails://localhost/media/token", ""), "media-server");
      assert.equal(videoSourceType("", "", "https://example.test/watch?v=one"), "remote-reference");
      assert.equal(videoSourceType("", "", ""), "unknown");
    },
  },
  {
    name: "includes complete bounded playback context",
    run: () => {
      const context = videoDiagnosticContext({
        trackId: "track-1",
        jobId: "job-2",
        sourceType: "media-server",
        mimeType: "video/webm",
        playerErrorCode: 4,
        browserEvent: "error",
        mode: "fullscreen",
        downloadMode: "on-demand",
        message: "video failed",
        sourcePathAvailable: true,
        remoteSourceAvailable: true,
      });
      assert.equal(context.trackId, "track-1");
      assert.equal(context.jobId, "job-2");
      assert.equal(context.sourceType, "media-server");
      assert.equal(context.mimeType, "video/webm");
      assert.equal(context.playerErrorCode, 4);
      assert.equal(context.browserEvent, "error");
      assert.equal(context.mode, "fullscreen");
      assert.equal(context.downloadMode, "on-demand");
      assert.ok(/^video-/.test(context.correlationId));
    },
  },
  {
    name: "redacts URL and header secrets and bounds error detail",
    run: () => {
      const detail = serializeVideoDiagnosticContext({
        message: `${"x".repeat(800)} Authorization: Bearer secret https://example.test/video?token=secret`,
      });
      assert.ok(detail.length <= 4096);
      assert.ok(!/secret/.test(detail));
      assert.ok(!/https:\/\//.test(detail));
      assert.ok(/redacted-url/.test(detail));
      assert.ok(/redacted/.test(detail));
    },
  },
];

for (const test of tests) {
  test.run();
  console.log(`ok - ${test.name}`);
}
