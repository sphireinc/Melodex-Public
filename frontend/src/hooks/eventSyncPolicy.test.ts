import assert from "node:assert/strict";
import { shouldUseStatePolling, targetedEventPolicy } from "./eventSyncPolicy";

type TestCase = { name: string; run: () => void };

const tests: TestCase[] = [
  {
    name: "packaged runtime event subscriptions disable state polling",
    run: () => {
      assert.equal(shouldUseStatePolling(true), false);
    },
  },
  {
    name: "development fallback keeps state polling when events are unavailable",
    run: () => {
      assert.equal(shouldUseStatePolling(false), true);
    },
  },
  {
    name: "unknown and older job-progress events follow the documented bridge-order policy",
    run: () => {
      const knownJobs = new Set(["job-1"]);
      const olderProgress = {
        jobId: "job-1",
        downloadProgress: 12,
        metadataProgress: 8,
        lyricsProgress: 0,
        status: "running",
      };

      assert.deepEqual(targetedEventPolicy("job-progress", olderProgress, knownJobs), {
        apply: true,
        reason: "ordered-targeted-event",
      });
      assert.deepEqual(targetedEventPolicy("job-progress", { ...olderProgress, jobId: "missing" }, knownJobs), {
        apply: false,
        reason: "unknown-target",
      });
      assert.deepEqual(targetedEventPolicy("job-progress", { status: "running" }, knownJobs), {
        apply: false,
        reason: "invalid-payload",
      });
    },
  },
  {
    name: "older playback events are accepted only when they satisfy the backend contract",
    run: () => {
      const olderPlayback = playbackState(12);
      assert.deepEqual(targetedEventPolicy("playback", olderPlayback), {
        apply: true,
        reason: "ordered-targeted-event",
      });
      assert.deepEqual(targetedEventPolicy("playback", { ...olderPlayback, currentTime: "12" }), {
        apply: false,
        reason: "invalid-payload",
      });
    },
  },
  {
    name: "older structural snapshots are authoritative and incomplete snapshots are ignored",
    run: () => {
      const olderState = structuralState(3);
      assert.deepEqual(targetedEventPolicy("structural", olderState), {
        apply: true,
        reason: "authoritative-structural-snapshot",
      });
      assert.deepEqual(targetedEventPolicy("structural", { jobs: olderState.jobs }), {
        apply: false,
        reason: "invalid-payload",
      });
    },
  },
];

let failures = 0;
for (const test of tests) {
  try {
    test.run();
    console.log(`ok - ${test.name}`);
  } catch (error) {
    failures++;
    console.error(`not ok - ${test.name}`);
    console.error(error);
  }
}

if (failures > 0) {
  throw new Error(`${failures} event sync policy test(s) failed`);
}

console.log(`event sync policy tests passed (${tests.length})`);

function playbackState(currentTime: number) {
  return {
    currentTrackId: "track-1",
    currentTrackPath: "/music/track-1.mp3",
    queue: ["track-1"],
    queueSource: "library",
    isPlaying: true,
    isLoading: false,
    currentTime,
    duration: 180,
    volume: 0.8,
    muted: false,
    shuffleEnabled: false,
    repeatMode: "off",
    error: "",
  };
}

function structuralState(jobCount: number) {
  return {
    settings: {},
    stats: {},
    health: {},
    workers: {},
    jobs: Array.from({ length: jobCount }, (_, index) => ({ id: `job-${index}` })),
    libraryTracks: [],
    recentTracks: [],
    playlists: [],
    playback: playbackState(3),
  };
}
