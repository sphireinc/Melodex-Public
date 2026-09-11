import assert from "node:assert/strict";
import { playbackTimeAt } from "../components/Queue/playbackClock";
import type { AppState, BrowseContext, Job, TrackRecord } from "../types";
import {
  aiStatusAlertClass,
  browserCookieOptions,
  currentImportQueueJob,
  getArtworkClickAction,
  filterTracks,
  getTracksForBrowseContext,
  groupAlbums,
  groupArtists,
  formatMaybeNumber,
  formatYesNoUnknown,
  hasAIContext,
  importCurrentLabel,
  jobActiveProcessLabel,
  metadataLinkIconLabel,
  normalizeAIContext,
  playlistEntriesFromRecord,
  playlistTrackCountFromSet,
  playlistTracksFromRecord,
  safeSplitImportUrls,
  queueSummaryCounts,
  patchJobProgress,
  selectedBrowseAlbum,
  selectedBrowseArtist,
  workerSummaryLabel,
} from "./viewHelpers";

type TestCase = { name: string; run: () => void };

const tracks = [
  track("1", "Artist A", "Album A", "Song A", "rock", "1999"),
  track("2", "Artist A", "Album A", "Song B", "rock", "1999"),
  track("3", "Artist B", "Album B", "Song C", "pop", "2001"),
];

const tests: TestCase[] = [
  {
    name: "browse context filtering narrows tracks correctly",
    run: () => {
      const artistContext: BrowseContext = { mode: "artist", artistName: "Artist A", artistKey: "Artist A" };
      const albumContext: BrowseContext = {
        mode: "album",
        artistName: "Artist A",
        artistKey: "Artist A",
        albumTitle: "Album A",
        albumKey: "Artist A::Album A",
      };

      assert.deepEqual(
        getTracksForBrowseContext(tracks, artistContext).map((track) => track.id),
        ["1", "2"],
      );
      assert.deepEqual(
        getTracksForBrowseContext(tracks, albumContext).map((track) => track.id),
        ["1", "2"],
      );

      const artists = groupArtists(tracks);
      const albums = groupAlbums(tracks);
      assert.equal(selectedBrowseArtist(artists, artistContext)?.artist, "Artist A");
      assert.equal(selectedBrowseAlbum(albums, albumContext)?.album, "Album A");
    },
  },
  {
    name: "track filters and queue summaries stay consistent",
    run: () => {
      const filtered = filterTracks(tracks, "Song", {
        lyrics: false,
        needsReview: false,
        missingMetadata: false,
        genre: "",
        year: "",
      });
      assert.equal(filtered.length, 3);

      const genreFiltered = filterTracks(tracks, "", {
        lyrics: false,
        needsReview: false,
        missingMetadata: false,
        genre: "rock",
        year: "1999",
      });
      assert.deepEqual(
        genreFiltered.map((track) => track.id),
        ["1", "2"],
      );

      const summary = queueSummaryCounts([
        job("1", "running"),
        job("2", "queued"),
        job("3", "completed"),
        job("4", "failed"),
      ]);
      assert.deepEqual(summary, { activelyProcessing: 1, done: 2, todo: 1 });
    },
  },
  {
    name: "playlist helpers preserve order and missing entries without repeated scans",
    run: () => {
      const playlist = {
        id: "playlist-1",
        name: "Fixture playlist",
        description: "",
        createdAt: "2026-01-01T00:00:00Z",
        updatedAt: "2026-01-01T00:00:00Z",
        trackIds: ["3", "missing", "1"],
      };
      assert.deepEqual(
        playlistTracksFromRecord(playlist, tracks).map((track) => track.id),
        ["3", "1"],
      );
      assert.deepEqual(
        playlistEntriesFromRecord(playlist, tracks).map((entry) => ({ trackID: entry.trackID, id: entry.track?.id ?? null })),
        [
          { trackID: "3", id: "3" },
          { trackID: "missing", id: null },
          { trackID: "1", id: "1" },
        ],
      );
      assert.equal(playlistTrackCountFromSet(playlist, new Set(["1", "3"])), 2);
    },
  },
  {
    name: "queue selection, import labels, and status helpers remain readable",
    run: () => {
      const current = currentImportQueueJob([
        job("later", "queued", "2024-06-01T10:05:00Z"),
        job("running", "running", "2024-06-01T10:06:00Z"),
        job("older", "queued", "2024-06-01T10:00:00Z"),
      ]);
      assert.equal(current?.id, "running");

      assert.equal(importCurrentLabel(null), "Idle");
      assert.equal(
        importCurrentLabel(job("p1", "running", "2024-06-01T10:00:00Z", "Playlist 1", 10, 0, 3)),
        "Playlist 1 · 10/3",
      );

      assert.equal(
        workerSummaryLabel({ workers: { activeWorkers: 2, maxWorkers: 3, videoActive: 0, videoQueued: 0 } }),
        "2 / 3 workers",
      );
      assert.equal(
        workerSummaryLabel({
          workers: {
            activeWorkers: 1,
            maxWorkers: 2,
            videoActive: 1,
            videoQueued: 2,
            blockedReason: "video-capacity",
          },
        }),
        "1 / 2 workers · Blocked: video capacity reached",
      );
      assert.equal(aiStatusAlertClass({ settings: { apiKeyConfigured: false }, aiStatus: "Ready" }), "missing");
      assert.equal(aiStatusAlertClass({ settings: { apiKeyConfigured: true }, aiStatus: "Ready" }), "ready");
      assert.deepEqual(safeSplitImportUrls("a\nb,c"), ["a", "b", "c"]);
      assert.equal(jobActiveProcessLabel(job("queued", "queued", "2024-06-01T10:00:00Z")), "Queued");
      assert.equal(
        jobActiveProcessLabel(job("playlist", "queued", "2024-06-01T10:00:00Z", "Playlist 1", 10, 0, 3, "playlist")),
        "Queued playlist",
      );
      assert.deepEqual(browserCookieOptions.slice(0, 3), ["brave", "chrome", "chromium"]);
      assert.equal(formatMaybeNumber(null), "—");
      assert.equal(formatMaybeNumber(12), "12");
      assert.equal(formatYesNoUnknown(undefined), "Unknown");
      assert.equal(formatYesNoUnknown(true), "Yes");
      assert.equal(formatYesNoUnknown(false), "No");
      assert.equal(metadataLinkIconLabel("YouTube Music"), "▶M");
      const normalized = normalizeAIContext({
        file_name: "file.mp3",
        source_url: "https://example.com",
        duration_seconds: 123,
        has_transcript: true,
      });
      assert.equal(normalized?.fileName, "file.mp3");
      assert.equal(normalized?.sourceUrl, "https://example.com");
      assert.equal(normalized?.durationSeconds, "123");
      assert.equal(normalized?.hasTranscript, "Yes");
      assert.equal(hasAIContext(normalized), true);
    },
  },
  {
    name: "album art click actions respect downloaded video state first",
    run: () => {
      const baseTrack = track("video", "Artist", "Album", "Song", "rock", "1999");
      assert.equal(getArtworkClickAction(baseTrack, true), "play-video");
      assert.equal(
        getArtworkClickAction({ ...baseTrack, sourceRef: "https://example.com/watch?v=123" }, false),
        "download-video",
      );
      assert.equal(getArtworkClickAction({ ...baseTrack, sourceRef: "" }, false), "none");
      assert.equal(getArtworkClickAction(null, false), "none");
    },
  },
  {
    name: "playback clock advances from its base and respects duration bounds",
    run: () => {
      const clock = { trackID: "track-1", baseTime: 12, startedAt: 1_000 };
      assert.equal(playbackTimeAt(2_500, clock, 0), 13.5);
      assert.equal(playbackTimeAt(2_500, clock, 13), 13);
      assert.equal(playbackTimeAt(500, clock, 0), 12);
    },
  },
  {
    name: "job progress events patch only the matching job",
    run: () => {
      const state: AppState = {
        settings: {
          libraryRoot: "~/Music/Melodex Music",
          aiBaseUrl: "",
          aiModel: "",
          provider: "",
          apiKeyConfigured: false,
          updateManifestUrl: "",
          ytDlpPath: "",
          ffmpegPath: "",
          ytDlpCookiesPath: "",
          ytDlpCookiesFromBrowser: "",
          videoDownloadMode: "on-demand",
          downloadMusicVideo: false,
          keepOriginalAudio: false,
          maxConcurrentDownloads: 1,
          maxConcurrentVideoDownloads: 1,
          maxConcurrentEnrichmentRequests: 1,
          maxConcurrentLyricsRequests: 1,
          throttleOnYtdlpBotErrors: true,
        },
        stats: {
          trackCount: 0,
          artistCount: 0,
          albumCount: 0,
          jobCount: 2,
          pendingJobs: 2,
          failedJobs: 0,
        },
        health: {
          totalTracks: 0,
          readyTracks: 0,
          unprocessedTracks: 0,
          missingAudio: 0,
          missingLyrics: 0,
          missingTimed: 0,
          missingMetadata: 0,
        },
        workers: {
          activeWorkers: 0,
          maxWorkers: 2,
          videoActive: 0,
          videoQueued: 0,
        },
        genreBuckets: [],
        yearBuckets: [],
        buildInfo: {
          appVersion: "",
          buildNumber: "",
          gitCommit: "",
          buildTime: "",
          releaseChannel: "",
          goVersion: "",
          settingsSchemaVersion: 0,
          catalogSchemaVersion: 0,
          libraryCacheSchemaVersion: 0,
          playlistSchemaVersion: 0,
          importHistorySchemaVersion: 0,
          trackMetadataSchemaVersion: 0,
        },
        updateInfo: {
          manifestUrl: "",
          currentVersion: "",
          latestVersion: "",
          releaseNotes: "",
          downloadUrl: "",
          platform: "",
          available: false,
          mandatory: false,
          minimumVersion: "",
          status: "",
          error: "",
        },
        diagnostics: {
          logPath: "",
          lastExportPath: "",
          bundleStatus: "",
          bundleMessage: "",
        },
        aiStatus: "",
        jobs: [job("1", "queued"), job("2", "running")],
        importHistory: [],
        libraryTracks: [],
        recentTracks: [],
        playlists: [],
        playback: {
          currentTrackId: "",
          currentTrackPath: "",
          queue: [],
          queueSource: "manual",
          isPlaying: false,
          isLoading: false,
          currentTime: 0,
          duration: 0,
          volume: 0.84,
          muted: false,
          shuffleEnabled: false,
          repeatMode: "off",
          error: "",
        },
        promptFiles: [],
        toolStatus: {
          ytDlp: false,
          ffmpeg: false,
          ai: false,
        },
        rootInfo: {
          libraryRoot: "",
          appDataDir: "",
          incomingDir: "",
          libraryDir: "",
          cacheDir: "",
        },
        windowFullscreen: false,
      };
      const next = patchJobProgress(state, {
        jobId: "2",
        downloadProgress: 70,
        metadataProgress: 40,
        lyricsProgress: 0,
        status: "running",
      });
      assert.ok(next);
      const queuedJob = next!.jobs[0]!;
      const runningJob = next!.jobs[1]!;
      assert.equal(queuedJob.status, "queued");
      assert.equal(runningJob.downloadProgress, 70);
      assert.equal(runningJob.metadataProgress, 40);
      assert.equal(runningJob.status, "running");
      assert.equal(next?.stats.jobCount, 2);
      assert.equal(next?.stats.pendingJobs, 2);
      assert.equal(next?.stats.failedJobs, 0);
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
  throw new Error(`${failures} frontend helper test(s) failed`);
}

console.log(`frontend helper tests passed (${tests.length})`);

function track(id: string, artist: string, album: string, title: string, genre: string, year: string): TrackRecord {
  return {
    id,
    artist,
    album,
    title,
    genre,
    year,
    hasLyrics: true,
    needsReview: false,
    duration: 180,
    lyricsIncluded: true,
    bundlePath: "",
    storageDir: "",
    audioPath: "",
    lyricsPath: "",
    lrcPath: "",
    artworkPath: "",
    videoPath: "",
    metadataPath: "",
    hash: "",
    createdAt: new Date("2024-06-01T10:00:00Z").toISOString(),
    sourceKind: "url",
    sourceRef: "",
    confidence: "high",
    metadataConfidence: "high",
    enrichmentConfidence: "high",
    releaseType: "album",
    isrc: "",
    artistLinks: {},
    albumLinks: {},
    songLinks: {},
    artistTrivia: [],
    albumTrivia: [],
    songTrivia: [],
    tidbits: [],
    sources: [],
    songMeaning: "",
    metadataNotes: "",
    lyricsConfidence: "high",
    lyricsSource: "lrclib",
    lyricsSourceLoc: "",
    timedLyricsConfidence: "low",
    timedLyricsGranularity: "none",
    hasTimedLyrics: false,
    sourceTitle: title,
    sourceChannel: "",
    aiProvider: "",
    aiModel: "",
    aiRan: true,
    aiStatus: "success",
    aiMessage: "",
    aiContext: {},
    generatedAt: new Date("2024-06-01T10:00:00Z").toISOString(),
    metadataSize: 0,
    metadataModTime: new Date("2024-06-01T10:00:00Z").toISOString(),
    artworkDataUrl: "",
    artworkMediaUrl: "",
    artworkUrl: "",
    videoUrl: "",
  } as TrackRecord;
}

function job(
  id: string,
  status: string,
  createdAt = "2024-06-01T10:00:00Z",
  playlistTitle = "",
  playlistProcessedItems = 0,
  playlistIndex = 0,
  playlistTotalItems = 0,
  kind: "url" | "playlist" = "url",
): Job {
  return {
    id,
    kind,
    input: `https://example.com/${id}`,
    status,
    detail: status,
    createdAt,
    playlistTitle,
    playlistProcessedItems,
    playlistIndex,
    playlistTotalItems,
  } as Job;
}
