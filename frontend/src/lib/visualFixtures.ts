import type { AppState, Job, Playlist, TrackRecord } from "../types";

const artworkDataUrl =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 800 800'%3E%3Cdefs%3E%3CradialGradient id='g' cx='30%25' cy='20%25'%3E%3Cstop stop-color='%23f4b860'/%3E%3Cstop offset='.48' stop-color='%23a63d70'/%3E%3Cstop offset='1' stop-color='%23101831'/%3E%3C/radialGradient%3E%3C/defs%3E%3Crect width='800' height='800' fill='url(%23g)'/%3E%3Ccircle cx='590' cy='230' r='170' fill='%23f7e8c5' fill-opacity='.18'/%3E%3Cpath d='M0 620 220 430l110 90 170-180 300 300v160H0Z' fill='%230b1028' fill-opacity='.76'/%3E%3Ctext x='58' y='710' fill='white' font-family='sans-serif' font-size='62' font-weight='700'%3EMELODEX%3C/text%3E%3C/svg%3E";

function fixtureTrack(
  base: AppState,
  input: Partial<TrackRecord> & Pick<TrackRecord, "id" | "artist" | "album" | "title">,
): TrackRecord {
  return {
    genre: input.genre ?? "Alternative",
    year: input.year ?? "2025",
    hasLyrics: input.hasLyrics ?? true,
    needsReview: input.needsReview ?? false,
    duration: input.duration ?? 238,
    lyricsIncluded: input.lyricsIncluded ?? true,
    confidence: input.confidence ?? "high",
    bundlePath: input.bundlePath ?? `fixture://${input.id}/bundle`,
    storageDir: input.storageDir ?? `fixture://${input.id}/album`,
    audioPath: input.audioPath ?? `fixture://${input.id}/audio.mp3`,
    lyricsPath: input.lyricsPath ?? `fixture://${input.id}/lyrics.txt`,
    lrcPath: input.lrcPath ?? `fixture://${input.id}/lyrics.lrc`,
    artworkPath: input.artworkPath ?? `fixture://${input.id}/artwork.svg`,
    videoPath: input.videoPath,
    artworkDataUrl: input.artworkDataUrl ?? artworkDataUrl,
    artworkMediaUrl: input.artworkMediaUrl,
    artworkUrl: input.artworkUrl,
    videoUrl: input.videoUrl,
    metadataPath: input.metadataPath ?? `fixture://${input.id}/metadata.json`,
    metadataConfidence: input.metadataConfidence ?? "high",
    enrichmentConfidence: input.enrichmentConfidence ?? "medium",
    releaseType: input.releaseType ?? "album",
    isrc: input.isrc,
    artistLinks: input.artistLinks ?? {
      officialWebsite: "https://example.com/artist",
      musicBrainz: "https://musicbrainz.org",
    },
    albumLinks: input.albumLinks ?? { spotify: "https://open.spotify.com", wikipedia: "https://wikipedia.org" },
    songLinks: input.songLinks ?? { youtubeMusic: "https://music.youtube.com" },
    artistTrivia: input.artistTrivia ?? ["Fixture artist focused on independent releases and careful cataloging."],
    albumTrivia: input.albumTrivia ?? ["Fixture album used for deterministic Melodex visual checks."],
    songTrivia: input.songTrivia ?? ["This is synthetic demo metadata and contains no external content."],
    songMeaning: input.songMeaning ?? "A short, deliberately generic interpretation for visual testing.",
    tidbits: input.tidbits ?? ["Visual fixture", "No network calls"],
    sources: input.sources ?? ["https://example.com/source"],
    metadataNotes: input.metadataNotes ?? "Fixture metadata; never written to the user library.",
    lyricsConfidence: input.lyricsConfidence ?? "high",
    lyricsSource: input.lyricsSource ?? "fixture",
    lyricsSourceLoc: input.lyricsSourceLoc ?? "fixture://lyrics",
    timedLyricsConfidence: input.timedLyricsConfidence ?? "medium",
    timedLyricsGranularity: input.timedLyricsGranularity ?? "line",
    hasTimedLyrics: input.hasTimedLyrics ?? true,
    sourceTitle: input.sourceTitle ?? input.title,
    sourceChannel: input.sourceChannel ?? "Melodex fixture channel",
    aiProvider: input.aiProvider ?? "fixture",
    aiModel: input.aiModel ?? "fixture-model",
    aiRan: input.aiRan ?? true,
    aiStatus: input.aiStatus ?? "Fixture data",
    aiMessage: input.aiMessage,
    aiContext: input.aiContext,
    generatedAt: input.generatedAt ?? "2026-08-03T12:00:00Z",
    sourceKind: input.sourceKind ?? "fixture",
    sourceRef: input.sourceRef ?? `fixture://${input.id}`,
    durationSeconds: input.durationSeconds ?? 238,
    metadataSize: input.metadataSize ?? 512,
    metadataModTime: input.metadataModTime ?? "2026-08-03T12:00:00Z",
    hash: input.hash ?? `fixture-hash-${input.id}`,
    createdAt: input.createdAt ?? "2026-08-03T12:00:00Z",
    ...input,
  };
}

function fixtureJob(id: string, status: string, detail: string, trackId?: string): Job {
  return {
    id,
    kind: "import",
    input: "fixture://visual-demo",
    sourceUrl: "https://example.com/owned-media",
    sourceRoot: "fixture://visual-demo",
    downloadVideo: true,
    trackId,
    status,
    detail,
    error: status === "failed" ? "Fixture failure shown for recovery-state review." : undefined,
    stageStatuses: {
      download: status === "done" ? "complete" : "processing",
      metadata: status === "done" ? "complete" : "queued",
      lyrics: status === "done" ? "complete" : "queued",
      artwork: status === "done" ? "complete" : "queued",
      video: status === "done" ? "complete" : "queued",
      finalize: status === "done" ? "complete" : "queued",
    },
    downloadProgress: status === "done" ? 1 : status === "failed" ? 0.42 : 0.68,
    metadataProgress: status === "done" ? 1 : 0.25,
    lyricsProgress: status === "done" ? 1 : 0,
    resultTitle: "Fixture track",
    resultArtist: "The Fixtures",
    resultAlbum: "Deterministic UI",
    createdAt: "2026-08-03T12:00:00Z",
    startedAt: "2026-08-03T12:01:00Z",
  };
}

export function createVisualFixtureState(base: AppState): AppState {
  const tracks = [
    fixtureTrack(base, {
      id: "fixture-signal",
      artist: "The Fixtures",
      album: "Deterministic UI",
      title: "Signal Through the Static",
      videoPath: "fixture://fixture-signal/video.mp4",
      videoUrl: "https://example.com/fixture-video",
      isrc: "US-FIX-25000001",
    }),
    fixtureTrack(base, {
      id: "fixture-afterglow",
      artist: "The Fixtures",
      album: "Deterministic UI",
      title: "Afterglow Index",
      genre: "Electronic",
      year: "2024",
      duration: 194,
    }),
    fixtureTrack(base, {
      id: "fixture-night-drive",
      artist: "Night Drive Unit",
      album: "Small Hours",
      title: "Small Hours",
      genre: "Dream Pop",
      year: "2023",
      duration: 281,
      needsReview: true,
      metadataConfidence: "medium",
    }),
    fixtureTrack(base, {
      id: "fixture-quiet-room",
      artist: "Night Drive Unit",
      album: "Small Hours",
      title: "Quiet Room",
      genre: "Dream Pop",
      year: "2023",
      duration: 226,
      hasLyrics: false,
      lyricsIncluded: false,
      hasTimedLyrics: false,
    }),
  ];
  const jobs = [
    fixtureJob("fixture-processing", "processing", "Downloading audio and preparing metadata", tracks[0]!.id),
    fixtureJob("fixture-failed", "failed", "Import failed and can be retried", tracks[2]!.id),
    fixtureJob("fixture-done", "done", "Imported and enriched successfully", tracks[1]!.id),
  ];
  const playlists: Playlist[] = [
    {
      id: "fixture-playlist",
      name: "Visual Review Set",
      description: "Synthetic playlist for deterministic screenshots.",
      createdAt: "2026-08-03T12:00:00Z",
      updatedAt: "2026-08-03T12:00:00Z",
      trackIds: tracks.map((track) => track.id),
    },
  ];
  return {
    ...base,
    settings: {
      ...base.settings,
      apiKeyConfigured: true,
    },
    stats: {
      ...base.stats,
      trackCount: tracks.length,
      artistCount: 2,
      albumCount: 2,
      jobCount: jobs.length,
      pendingJobs: 1,
      failedJobs: 1,
    },
    health: {
      ...base.health,
      totalTracks: tracks.length,
      readyTracks: 3,
      unprocessedTracks: 1,
      missingLyrics: 1,
      missingTimed: 1,
      missingMetadata: 1,
    },
    workers: {
      ...base.workers,
      activeWorkers: 1,
      maxWorkers: 2,
      videoActive: 0,
      videoQueued: 1,
    },
    genreBuckets: [
      { value: "Alternative", count: 2 },
      { value: "Dream Pop", count: 2 },
      { value: "Electronic", count: 1 },
    ],
    yearBuckets: [
      { value: "2025", count: 1 },
      { value: "2024", count: 1 },
      { value: "2023", count: 2 },
    ],
    jobs,
    importHistory: Array.from({ length: 10_000 }, (_, index) => ({
      url: `https://example.test/large-fixture/import/${String(index).padStart(5, "0")}`,
      completedAt: "2026-08-09T00:00:00Z",
    })),
    libraryTracks: tracks,
    recentTracks: tracks.slice(0, 3),
    playlists,
    playback: {
      ...base.playback,
      currentTrackId: tracks[0]!.id,
      currentTrackPath: tracks[0]!.audioPath,
      queue: tracks.map((track) => track.id),
      queueSource: "Visual Review Set",
      isPlaying: false,
      currentTime: 73,
      duration: tracks[0]!.duration ?? 238,
      volume: 0.84,
    },
    aiStatus: "Configured for fixture review",
    toolStatus: { ytDlp: true, ffmpeg: true, ai: true },
    windowFullscreen: false,
  };
}

/**
 * A larger development-only fixture for profiling bounded library surfaces.
 * It intentionally reuses immutable seed metadata references so the fixture
 * stresses grouping/rendering without creating 10,000 duplicate artwork or
 * enrichment payloads in memory.
 */
export function createLargeVisualFixtureState(base: AppState): AppState {
  const seedState = createVisualFixtureState(base);
  const seed = seedState.libraryTracks[0];
  if (!seed) return seedState;

  const trackCount = 10_000;
  const artistCount = 2_000;
  const albumCount = 1_000;
  const playlistCount = 100;
  const tracks = Array.from({ length: trackCount }, (_, index) => {
    const artist = `Large Fixture Artist ${String(index % artistCount).padStart(4, "0")}`;
    const album = `Large Fixture Album ${String(index % albumCount).padStart(4, "0")}`;
    const id = `large-fixture-track-${String(index).padStart(5, "0")}`;
    return {
      ...seed,
      id,
      artist,
      album,
      title: `Large Fixture Track ${String(index).padStart(5, "0")}`,
      genre: `Fixture Genre ${index % 10}`,
      year: String(2000 + (index % 25)),
      audioPath: `fixture://${id}/audio.mp3`,
      storageDir: `fixture://${id}/album`,
      metadataPath: `fixture://${id}/metadata.json`,
      artworkPath: "",
      artworkDataUrl: "",
      artworkMediaUrl: "",
      videoPath: undefined,
      videoUrl: undefined,
      sourceRef: `fixture://${id}`,
    };
  });
  const playlists = Array.from({ length: playlistCount }, (_, playlistIndex): Playlist => {
    const id = `large-fixture-playlist-${String(playlistIndex).padStart(3, "0")}`;
    const trackIds = Array.from({ length: 100 }, (_, trackIndex) => {
      const sourceIndex = (playlistIndex * 100 + trackIndex) % trackCount;
      return tracks[sourceIndex]!.id;
    });
    return {
      id,
      name: `Large Fixture Playlist ${String(playlistIndex).padStart(3, "0")}`,
      description: "Synthetic profiling fixture; never written to the user library.",
      createdAt: "2026-08-09T00:00:00Z",
      updatedAt: "2026-08-09T00:00:00Z",
      trackIds,
    };
  });
  const jobs = Array.from({ length: 100 }, (_, index) =>
    fixtureJob(
      `large-fixture-job-${String(index).padStart(3, "0")}`,
      index % 5 === 0 ? "failed" : index % 3 === 0 ? "processing" : "done",
      "Synthetic profiling job",
      tracks[index]!.id,
    ),
  );

  return {
    ...seedState,
    stats: {
      ...seedState.stats,
      trackCount,
      artistCount,
      albumCount,
      jobCount: jobs.length,
      pendingJobs: jobs.filter((job) => job.status === "processing").length,
      failedJobs: jobs.filter((job) => job.status === "failed").length,
    },
    health: {
      ...seedState.health,
      totalTracks: trackCount,
      readyTracks: trackCount,
      unprocessedTracks: 0,
      missingLyrics: 0,
      missingTimed: 0,
      missingMetadata: 0,
    },
    genreBuckets: Array.from({ length: 10 }, (_, index) => ({ value: `Fixture Genre ${index}`, count: trackCount / 10 })),
    yearBuckets: Array.from({ length: 25 }, (_, index) => ({ value: String(2000 + index), count: 400 })),
    jobs,
    libraryTracks: tracks,
    recentTracks: tracks.slice(0, 100),
    playlists,
    playback: {
      ...seedState.playback,
      currentTrackId: tracks[0]!.id,
      currentTrackPath: tracks[0]!.audioPath,
      queue: tracks.slice(0, 100).map((track) => track.id),
      queueSource: "Large Fixture Playlist 000",
    },
  };
}

export const visualFixtureVideo: { url: string; mimeType: string } = {
  // Intentionally invalid media content: it deterministically exercises the visible video-error recovery UI.
  url: "data:video/mp4;base64,AAAA",
  mimeType: "video/mp4",
};
