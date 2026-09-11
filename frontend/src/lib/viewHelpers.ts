import type { AppState, BrowseContext, Job, JobProgressEvent, MetadataLinks, Playlist, TrackRecord } from "../types";

export function capitalize(value: string) {
  if (!value) return "";
  return value.charAt(0).toUpperCase() + value.slice(1);
}

export function pageTitle(view: string) {
  switch (view) {
    case "library":
      return "Library";
    case "index":
      return "Index";
    case "artists":
      return "Artists";
    case "albums":
      return "Albums";
    case "songs":
      return "Songs";
    case "import":
      return "Import Music";
    case "processing":
      return "Processing";
    case "playlists":
      return "Playlists";
    case "settings":
      return "Settings";
    default:
      return "Melodex";
  }
}

export function pageSubtitle(view: string) {
  switch (view) {
    case "library":
      return "Browse your local collection by album, artist, or song.";
    case "index":
      return "Search, filter, and inspect the library index.";
    case "artists":
      return "Drill into artists, then open the albums beneath them.";
    case "albums":
      return "Drill into albums, then open the songs beneath them.";
    case "songs":
      return "Inspect or play the songs in the current context.";
    case "import":
      return "Queue files or links into the pipeline.";
    case "processing":
      return "Queue and job progress.";
    case "playlists":
      return "Create, reorder, and manage playlists.";
    case "settings":
      return "Configure storage, AI, and downloader tools.";
    default:
      return "";
  }
}

export function formatDateTime(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

export function formatDuration(seconds: number) {
  if (!Number.isFinite(seconds) || seconds < 0) return "0:00";
  const total = Math.round(seconds);
  const minutes = Math.floor(total / 60);
  const remaining = total % 60;
  return `${minutes}:${remaining.toString().padStart(2, "0")}`;
}

export type AlbumGroup = {
  key: string;
  album: string;
  artist: string;
  artistKey: string;
  tracks: TrackRecord[];
  firstTrack: TrackRecord;
  trackCount: number;
  albumCount: number;
  sampleAlbums: string[];
};

export type ArtistGroup = {
  key: string;
  artist: string;
  artistKey: string;
  tracks: TrackRecord[];
  firstTrack: TrackRecord;
  trackCount: number;
  albumCount: number;
  sampleAlbums: string[];
};

export function trackDuration(track: TrackRecord) {
  return track.durationSeconds || track.duration || 0;
}

export function initialsForTrack(track?: TrackRecord | null) {
  if (!track) return "M";
  const source = `${track.artist || ""} ${track.title || ""}`.trim();
  if (!source) return "M";
  return source
    .split(/\s+/)
    .slice(0, 2)
    .map((part) => part.charAt(0).toUpperCase())
    .join("");
}

export function groupAlbums(tracks: TrackRecord[]): AlbumGroup[] {
  const groups = new Map<string, AlbumGroup>();
  for (const track of tracks) {
    const key = `${track.artist}::${track.album}`;
    const existing = groups.get(key);
    if (existing) {
      existing.tracks.push(track);
      continue;
    }
    groups.set(key, {
      key,
      album: track.album || "Unknown",
      artist: track.artist || "Unknown",
      artistKey: track.artist || "Unknown",
      tracks: [track],
      firstTrack: track,
      trackCount: 1,
      albumCount: 1,
      sampleAlbums: track.album ? [track.album] : [],
    });
  }
  const values = [...groups.values()];
  for (const group of values) {
    group.trackCount = group.tracks.length;
    group.albumCount = 1;
    group.sampleAlbums = [...new Set(group.tracks.map((track) => track.album).filter(Boolean))].slice(0, 3);
  }
  return values.sort((left, right) => left.artist.localeCompare(right.artist) || left.album.localeCompare(right.album));
}

export function groupArtists(tracks: TrackRecord[]): ArtistGroup[] {
  const groups = new Map<string, ArtistGroup>();
  for (const track of tracks) {
    const key = track.artist || "Unknown";
    const existing = groups.get(key);
    if (existing) {
      existing.tracks.push(track);
      continue;
    }
    groups.set(key, {
      key,
      artist: key,
      artistKey: key,
      tracks: [track],
      firstTrack: track,
      trackCount: 1,
      albumCount: 1,
      sampleAlbums: track.album ? [track.album] : [],
    });
  }
  const values = [...groups.values()];
  for (const group of values) {
    group.trackCount = group.tracks.length;
    group.albumCount = new Set(group.tracks.map((track) => track.album).filter(Boolean)).size;
    group.sampleAlbums = [...new Set(group.tracks.map((track) => track.album).filter(Boolean))].slice(0, 3);
  }
  return values.sort((left, right) => left.artist.localeCompare(right.artist));
}

export function selectedBrowseArtist(groups: ReturnType<typeof groupArtists>, context: BrowseContext) {
  if (context.mode !== "artist" && context.mode !== "album" && context.mode !== "songs") return null;
  const key = "artistKey" in context ? context.artistKey : "";
  return groups.find((group) => group.key === key) ?? null;
}

export function selectedBrowseAlbum(groups: ReturnType<typeof groupAlbums>, context: BrowseContext) {
  if (context.mode !== "album" && context.mode !== "songs") return null;
  const key = "albumKey" in context ? context.albumKey : "";
  return groups.find((group) => group.key === key) ?? null;
}

export function getTracksForBrowseContext(tracks: TrackRecord[], context: BrowseContext) {
  switch (context.mode) {
    case "library":
      return tracks;
    case "artist":
      return tracks.filter((track) => track.artist === context.artistName);
    case "album":
      return tracks.filter((track) => track.artist === context.artistName && track.album === context.albumTitle);
    case "songs":
      if (context.albumKey) {
        return tracks.filter((track) => `${track.artist}::${track.album}` === context.albumKey);
      }
      if (context.artistKey) {
        return tracks.filter((track) => track.artist === context.artistName);
      }
      return tracks;
    default:
      return tracks;
  }
}

export function filterTracks(
  tracks: TrackRecord[],
  search: string,
  filters: { lyrics: boolean; needsReview: boolean; missingMetadata: boolean; genre: string; year: string },
) {
  const query = search.trim().toLowerCase();
  return tracks.filter((track) => {
    const haystack = `${track.title} ${track.artist} ${track.album} ${track.genre} ${track.year ?? ""}`.toLowerCase();
    if (query && !haystack.includes(query)) return false;
    if (filters.lyrics && !track.hasLyrics) return false;
    if (filters.needsReview && !track.needsReview) return false;
    if (filters.missingMetadata && track.metadataConfidence === "high") return false;
    if (filters.genre && track.genre !== filters.genre) return false;
    if (filters.year && String(track.year ?? "") !== filters.year) return false;
    return true;
  });
}

export function playlistTracksFromRecord(playlist: Playlist, tracks: TrackRecord[]) {
  const tracksByID = new Map(tracks.map((track) => [track.id, track]));
  return playlist.trackIds.map((id) => tracksByID.get(id)).filter(Boolean) as TrackRecord[];
}

export function playlistTrackCountFromSet(playlist: Playlist, trackIDs: ReadonlySet<string>) {
  return playlist.trackIds.reduce((count, trackID) => count + (trackIDs.has(trackID) ? 1 : 0), 0);
}

export function playlistEntriesFromRecord(playlist: Playlist, tracks: TrackRecord[]) {
  const tracksByID = new Map(tracks.map((track) => [track.id, track]));
  return playlist.trackIds.map((trackID) => ({
    trackID,
    track: tracksByID.get(trackID) ?? null,
  }));
}

export function splitImportUrls(value: string) {
  return value
    .split(/[\n,]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export type ArtworkClickAction = "play-video" | "download-video" | "none";

export function getArtworkClickAction(track: TrackRecord | null, hasVideoDownloaded: boolean) {
  if (!track) return "none";
  if (hasVideoDownloaded) return "play-video";
  if (track.sourceRef.trim()) return "download-video";
  return "none";
}

export type MetadataLinkEntry = {
  label: string;
  url: string;
};

export function cleanStringList(values?: string[]) {
  return (values ?? []).map((value) => value.trim()).filter(Boolean);
}

export function metadataLinkEntries(links?: MetadataLinks | null): MetadataLinkEntry[] {
  if (!links) return [];
  const entries: MetadataLinkEntry[] = [];
  const push = (label: string, url?: string | null) => {
    if (url && url.trim()) {
      entries.push({ label, url: url.trim() });
    }
  };

  push("Official website", links.officialWebsite);
  push("Spotify", links.spotify);
  push("Apple Music", links.appleMusic);
  push("YouTube", links.youtube);
  push("YouTube Music", links.youtubeMusic);
  push("Instagram", links.instagram);
  push("X", links.x);
  push("Facebook", links.facebook);
  push("Bandcamp", links.bandcamp);
  push("SoundCloud", links.soundcloud);
  push("Wikipedia", links.wikipedia);
  push("MusicBrainz", links.musicBrainz);
  push("Genius", links.genius);
  return entries;
}

export function looksLikeUrl(value: string) {
  const trimmed = value.trim();
  return /^https?:\/\//i.test(trimmed);
}

export function formatSourceLabel(value: string) {
  const trimmed = value.trim();
  if (!trimmed) return trimmed;
  try {
    const parsed = new URL(trimmed);
    return parsed.hostname.replace(/^www\./i, "");
  } catch {
    return trimmed;
  }
}

export function metadataLinkIconKey(label: string) {
  return label.trim().toLowerCase();
}

export function metadataLinkIconLabel(label: string) {
  const key = metadataLinkIconKey(label);
  if (key.includes("apple")) return "";
  if (key.includes("bandcamp")) return "BC";
  if (key.includes("soundcloud")) return "SC";
  if (key.includes("genius")) return "G";
  if (key.includes("wikipedia")) return "W";
  if (key.includes("musicbrainz")) return "MB";
  if (key.includes("youtube music")) return "▶M";
  if (key === "youtube") return "▶";
  if (key.includes("spotify")) return "S";
  return "↗";
}

export const browserCookieOptions = [
  "brave",
  "chrome",
  "chromium",
  "edge",
  "firefox",
  "opera",
  "safari",
  "vivaldi",
  "whale",
];

export function safeSplitImportUrls(value: string) {
  return value
    .split(/[\n,]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function jobActiveProcessLabel(job: Job) {
  if (job.status === "queued") {
    return job.kind === "playlist" ? "Queued playlist" : "Queued";
  }
  if (job.status !== "running") {
    return "Processing";
  }
  const stages = job.stageStatuses ?? {};
  const candidates = [
    { label: "Downloading", status: stages.download },
    { label: "MusicBrainz", status: stages.musicBrainz },
    { label: "Metadata", status: stages.metadata },
    { label: "Lyrics", status: stages.lyrics },
    { label: "Artwork", status: stages.artwork },
    { label: "Video", status: stages.video },
    { label: "Finalizing", status: stages.finalize },
  ];
  for (const candidate of candidates) {
    const status = (candidate.status ?? "").trim().toLowerCase();
    if (!status) continue;
    if (status === "running" || status === "processing" || status === "downloading" || status === "matching") {
      return candidate.label;
    }
  }
  if ((job.downloadProgress ?? 0) < 100) return "Downloading";
  if ((job.metadataProgress ?? 0) < 100) return "Metadata";
  if ((job.lyricsProgress ?? 0) < 100) return "Lyrics";
  return "Finalizing";
}

function recalculateQueueTotals(jobs: Job[]) {
  const pending = jobs.filter((job) => job.status === "queued" || job.status === "running").length;
  const failed = jobs.filter((job) => job.status === "failed" || job.status === "stopped").length;
  return {
    jobCount: jobs.length,
    pendingJobs: pending,
    failedJobs: failed,
  };
}

export function patchJobProgress(state: AppState | null, event: JobProgressEvent) {
  if (!state || !event.jobId) return state;
  let changed = false;
  const jobs = state.jobs.map((job) => {
    if (job.id !== event.jobId) return job;
    changed = true;
    return {
      ...job,
      downloadProgress: event.downloadProgress,
      metadataProgress: event.metadataProgress,
      lyricsProgress: event.lyricsProgress,
      status: event.status || job.status,
      detail: event.detail ?? job.detail,
      stageStatuses: event.stageStatuses ?? job.stageStatuses,
      parentJobId: event.parentJobId || job.parentJobId,
      playlistTotalItems: event.playlistTotal ?? job.playlistTotalItems,
      playlistProcessedItems: event.playlistProcessed ?? job.playlistProcessedItems,
      playlistFailedItems: event.playlistFailed ?? job.playlistFailedItems,
    };
  });
  if (!changed) return state;
  const totals = recalculateQueueTotals(jobs);
  return {
    ...state,
    stats: {
      ...state.stats,
      jobCount: totals.jobCount,
      pendingJobs: totals.pendingJobs,
      failedJobs: totals.failedJobs,
    },
    jobs,
  };
}

export function shuffleList<T>(items: T[]) {
  const next = [...items];
  for (let index = next.length - 1; index > 0; index--) {
    const swap = Math.floor(Math.random() * (index + 1));
    const current = next[index]!;
    const chosen = next[swap]!;
    next[index] = chosen;
    next[swap] = current;
  }
  return next;
}

export function currentTracks(
  state: { libraryTracks: TrackRecord[]; recentTracks: TrackRecord[]; playback?: { queue: string[] } } | null,
) {
  if (!state) return [];
  return state.libraryTracks.length > 0 ? state.libraryTracks : state.recentTracks;
}

export function currentImportQueueJob(jobs: Job[]) {
  const importJobs = jobs.filter(
    (job) => (job.kind === "url" || job.kind === "playlist") && (job.status === "running" || job.status === "queued"),
  );
  const running = importJobs.find((job) => job.status === "running");
  if (running) return running;
  if (importJobs.length === 0) return null;
  return importJobs.reduce((oldest, current) => {
    const oldestTime = new Date(oldest.createdAt).getTime();
    const currentTime = new Date(current.createdAt).getTime();
    return currentTime < oldestTime ? current : oldest;
  });
}

export function queueSummaryCounts(jobs: Job[]) {
  const activelyProcessing = jobs.filter((job) => job.status === "running").length;
  const done = jobs.filter((job) => job.status !== "queued" && job.status !== "running").length;
  const todo = jobs.filter((job) => job.status === "queued").length;
  return { activelyProcessing, done, todo };
}

export function workerSummaryLabel(
  state: {
    workers?: {
      activeWorkers: number;
      maxWorkers: number;
      videoActive: number;
      videoQueued: number;
      blockedReason?: string;
      throttle?: { active: boolean };
    };
  } | null,
) {
  if (!state?.workers) return "0 / 0 workers";
  const parts = [`${state.workers.activeWorkers} / ${state.workers.maxWorkers} workers`];
  if (state.workers.blockedReason) {
    parts.push(`Blocked: ${workerBlockedReasonLabel(state.workers.blockedReason)}`);
  } else if (state.workers.throttle?.active) {
    parts.push("Throttled");
  }
  return parts.join(" · ");
}

export function workerBlockedReasonLabel(reason?: string) {
  switch (reason) {
    case "queue-paused":
      return "queue paused";
    case "video-capacity":
      return "video capacity reached";
    case "worker-capacity":
      return "worker capacity reached";
    case "throttled":
      return "external tool throttled";
    default:
      return reason || "waiting";
  }
}

export function aiStatusAlertClass(state: { aiStatus?: string; settings?: { apiKeyConfigured?: boolean } } | null) {
  if (!state) return "missing";
  if (!state.settings?.apiKeyConfigured) return "missing";
  const status = (state.aiStatus || "").toLowerCase();
  if (status.includes("missing") || status.includes("failed")) return "missing";
  return "ready";
}

export function importCurrentLabel(job: Job | null) {
  if (!job) return "Idle";
  if (job.playlistTitle) {
    return `${job.playlistTitle} · ${job.playlistProcessedItems ?? 0}/${job.playlistTotalItems ?? 0}`;
  }
  return job.input || job.sourceUrl || "Importing";
}

export function confidenceLabel(value?: string) {
  if (!value) return "Unknown";
  return value.charAt(0).toUpperCase() + value.slice(1);
}

export function badgeToneFromConfidence(value?: string) {
  switch ((value || "").toLowerCase()) {
    case "high":
      return "success";
    case "medium":
      return "warning";
    case "low":
      return "danger";
    default:
      return "neutral";
  }
}

export function badgeToneFromTrack(track: TrackRecord) {
  return badgeToneFromConfidence(track.metadataConfidence);
}

export function metadataLabel(track: TrackRecord) {
  return confidenceLabel(track.metadataConfidence || "unknown");
}

export function formatMaybeNumber(value?: number | null) {
  if (value === null || value === undefined) return "—";
  return String(value);
}

export function formatYesNoUnknown(value?: boolean) {
  if (value === undefined) return "Unknown";
  return value ? "Yes" : "No";
}

export function relativeOrAbsolute(path: string) {
  return path;
}

export type NormalizedAIContext = {
  fileName: string;
  sourceRef: string;
  sourceUrl: string;
  sourceKind: string;
  libraryRoot: string;
  sourceTitle: string;
  sourceUploader: string;
  sourceChannel: string;
  sourceDescriptionHint: string;
  durationSeconds: string;
  durationHint: string;
  hasTranscript: string;
  hasTimestampedTranscript: string;
  inputText: string;
  userContext: string;
  artistHint: string;
  albumHint: string;
  titleHint: string;
  yearHint: string;
  genreHint: string;
  trackNumberHint: string;
  resolvedTitle: string;
  resolvedArtist: string;
  resolvedAlbum: string;
  resolvedTrackNumber: string;
  resolvedYear: string;
  resolvedGenre: string;
  resolvedConfidence: string;
  resolvedNotes: string;
  targetArtistDir: string;
  targetAlbumDir: string;
  targetBaseName: string;
  targetAudioPath: string;
  targetLyricsPath: string;
  targetTimedLyricsPath: string;
  targetMetadataPath: string;
  notes: string;
};

export function normalizeAIContext(context?: unknown | null): NormalizedAIContext | null {
  if (!context) return null;
  const raw = context as Record<string, unknown>;
  const readString = (snake: string, legacy?: string) => {
    const value = raw[snake] ?? (legacy ? raw[legacy] : undefined);
    return typeof value === "string" ? value : "";
  };
  const readNumber = (snake: string, legacy?: string) => {
    const value = raw[snake] ?? (legacy ? raw[legacy] : undefined);
    return typeof value === "number" ? String(value) : "";
  };
  const readBool = (snake: string, legacy?: string) => {
    const value = raw[snake] ?? (legacy ? raw[legacy] : undefined);
    return typeof value === "boolean" ? (value ? "Yes" : "No") : "";
  };
  return {
    fileName: readString("file_name", "fileName"),
    sourceRef: readString("source_ref", "sourceRef"),
    sourceUrl: readString("source_url", "sourceURL"),
    sourceKind: readString("source_kind", "sourceKind"),
    libraryRoot: readString("library_root", "libraryRoot"),
    sourceTitle: readString("source_title", "sourceTitle"),
    sourceUploader: readString("source_uploader", "sourceUploader"),
    sourceChannel: readString("source_channel", "sourceChannel"),
    sourceDescriptionHint: readString("source_description_hint", "sourceDescriptionHint"),
    durationSeconds: readNumber("duration_seconds", "durationSeconds"),
    durationHint: readString("duration_hint", "durationHint"),
    hasTranscript: readBool("has_transcript", "hasTranscript"),
    hasTimestampedTranscript: readBool("has_timestamped_transcript", "hasTimestampedTranscript"),
    inputText: readString("input_text", "inputText"),
    userContext: readString("user_context", "userContext"),
    artistHint: readString("artist_hint", "artistHint"),
    albumHint: readString("album_hint", "albumHint"),
    titleHint: readString("title_hint", "titleHint"),
    yearHint: readString("year_hint", "yearHint"),
    genreHint: readString("genre_hint", "genreHint"),
    trackNumberHint: readString("track_number_hint", "trackNumberHint"),
    resolvedTitle: readString("resolved_title", "resolvedTitle"),
    resolvedArtist: readString("resolved_artist", "resolvedArtist"),
    resolvedAlbum: readString("resolved_album", "resolvedAlbum"),
    resolvedTrackNumber: readString("resolved_track_number", "resolvedTrackNumber"),
    resolvedYear: readString("resolved_year", "resolvedYear"),
    resolvedGenre: readString("resolved_genre", "resolvedGenre"),
    resolvedConfidence: readString("resolved_confidence", "resolvedConfidence"),
    resolvedNotes: readString("resolved_notes", "resolvedNotes"),
    targetArtistDir: readString("target_artist_dir", "targetArtistDir"),
    targetAlbumDir: readString("target_album_dir", "targetAlbumDir"),
    targetBaseName: readString("target_base_name", "targetBaseName"),
    targetAudioPath: readString("target_audio_path", "targetAudioPath"),
    targetLyricsPath: readString("target_lyrics_path", "targetLyricsPath"),
    targetTimedLyricsPath: readString("target_timed_lyrics_path", "targetTimedLyricsPath"),
    targetMetadataPath: readString("target_metadata_path", "targetMetadataPath"),
    notes: readString("notes"),
  };
}

export function hasAIContext(context?: unknown | null) {
  const normalized = normalizeAIContext(context);
  if (!normalized) return false;
  return Boolean(
    normalized.fileName ||
    normalized.sourceRef ||
    normalized.sourceUrl ||
    normalized.sourceKind ||
    normalized.libraryRoot ||
    normalized.sourceTitle ||
    normalized.sourceUploader ||
    normalized.sourceChannel ||
    normalized.sourceDescriptionHint ||
    normalized.durationSeconds ||
    normalized.durationHint ||
    normalized.hasTranscript ||
    normalized.hasTimestampedTranscript ||
    normalized.inputText ||
    normalized.userContext ||
    normalized.artistHint ||
    normalized.albumHint ||
    normalized.titleHint ||
    normalized.yearHint ||
    normalized.genreHint ||
    normalized.trackNumberHint ||
    normalized.resolvedTitle ||
    normalized.resolvedArtist ||
    normalized.resolvedAlbum ||
    normalized.resolvedTrackNumber ||
    normalized.resolvedYear ||
    normalized.resolvedGenre ||
    normalized.resolvedConfidence ||
    normalized.resolvedNotes ||
    normalized.targetArtistDir ||
    normalized.targetAlbumDir ||
    normalized.targetBaseName ||
    normalized.targetAudioPath ||
    normalized.targetLyricsPath ||
    normalized.targetTimedLyricsPath ||
    normalized.targetMetadataPath ||
    normalized.notes,
  );
}

export function jobTone(status: string) {
  switch (status) {
    case "running":
    case "queued":
      return "accent";
    case "done":
    case "completed":
      return "success";
    case "failed":
      return "danger";
    default:
      return "neutral";
  }
}

export function jobResultLabel(job: Job) {
  return job.detail || job.status;
}

export function jobPlaylistSummary(job: Job) {
  if (job.kind === "playlist" && !job.parentJobId) {
    const processed = job.playlistProcessedItems ?? 0;
    const total = job.playlistTotalItems ?? 0;
    const failed = job.playlistFailedItems ?? 0;
    const current = (job.playlistCurrentTitle ?? "").trim();
    const parts = [`${processed}/${total} processed`, `${failed} failed`];
    if (current) {
      parts.push(`Current: ${current}`);
    }
    return parts.join(" · ");
  }
  if (job.parentJobId) {
    const index = job.playlistIndex ?? 0;
    const total = job.playlistTotalItems ?? 0;
    const title = (job.playlistItemTitle ?? "").trim();
    const playlist = (job.playlistTitle ?? "").trim();
    const parts: string[] = [];
    if (index > 0 && total > 0) {
      parts.push(`Item ${index}/${total}`);
    }
    if (title) {
      parts.push(title);
    }
    if (playlist) {
      parts.push(`From ${playlist}`);
    }
    return parts.join(" · ");
  }
  return "";
}

export function isPlaylistRootJob(job: Job) {
  return job.kind === "playlist" && !job.parentJobId;
}

export function jobStageProgressItems(job: Job) {
  return [
    { label: "Downloading", value: job.downloadProgress ?? jobProgressFromStatus(job.status) },
    { label: "Metadata", value: job.metadataProgress ?? jobProgressFromStatus(job.status) },
    { label: "Lyrics", value: job.lyricsProgress ?? jobProgressFromStatus(job.status) },
  ];
}

export function fallbackStageStatus(
  job: Job,
  stage: "download" | "metadata" | "musicBrainz" | "lyrics" | "artwork" | "video" | "finalize",
) {
  const status = job.status.toLowerCase();
  const terminal = {
    download: "Downloaded",
    metadata: "Metadata discovered",
    musicBrainz: "Matched",
    lyrics: "Matched",
    artwork: "Downloaded",
    video: "Downloaded",
    finalize: "Completed",
  }[stage];
  if (stage === "video") {
    if (status === "failed") return "Failed";
    if (status === "stopped") return "Stopped";
    if (status === "completed") return "Downloaded";
    if (status === "running") return "Running";
    return "Queued";
  }
  const progress = {
    download: job.downloadProgress ?? 0,
    metadata: job.metadataProgress ?? 0,
    lyrics: job.lyricsProgress ?? 0,
    musicBrainz: job.metadataProgress ?? 0,
    artwork: job.metadataProgress ?? 0,
    video: job.downloadProgress ?? 0,
    finalize: Math.max(job.downloadProgress ?? 0, job.metadataProgress ?? 0, job.lyricsProgress ?? 0),
  }[stage];
  if (status === "failed") return "Failed";
  if (status === "stopped") return "Stopped";
  if (status === "completed") return terminal;
  if (status === "running") {
    if (progress >= 100) return terminal;
    if (progress > 0) return "Running";
  }
  return "Queued";
}

export function jobStageStatusItems(job: Job) {
  const stages = job.stageStatuses ?? {};
  return [
    {
      label: "Download",
      value: stages.download ?? fallbackStageStatus(job, "download"),
      tone: stageChipTone(stages.download ?? fallbackStageStatus(job, "download")),
    },
    {
      label: "Metadata",
      value: stages.metadata ?? fallbackStageStatus(job, "metadata"),
      tone: stageChipTone(stages.metadata ?? fallbackStageStatus(job, "metadata")),
    },
    {
      label: "MusicBrainz",
      value: stages.musicBrainz ?? fallbackStageStatus(job, "musicBrainz"),
      tone: stageChipTone(stages.musicBrainz ?? fallbackStageStatus(job, "musicBrainz")),
    },
    {
      label: "Lyrics",
      value: stages.lyrics ?? fallbackStageStatus(job, "lyrics"),
      tone: stageChipTone(stages.lyrics ?? fallbackStageStatus(job, "lyrics")),
    },
    {
      label: "Artwork",
      value: stages.artwork ?? fallbackStageStatus(job, "artwork"),
      tone: stageChipTone(stages.artwork ?? fallbackStageStatus(job, "artwork")),
    },
    {
      label: "Video",
      value: stages.video ?? fallbackStageStatus(job, "video"),
      tone: stageChipTone(stages.video ?? fallbackStageStatus(job, "video")),
    },
    {
      label: "Finalize",
      value: stages.finalize ?? fallbackStageStatus(job, "finalize"),
      tone: stageChipTone(stages.finalize ?? fallbackStageStatus(job, "finalize")),
    },
  ];
}

export function stageChipTone(value: string) {
  const normalized = value.trim().toLowerCase();
  if (!normalized || normalized === "queued") return "neutral";
  if (normalized === "running") return "warning";
  if (normalized === "failed" || normalized === "stopped") return "danger";
  if (normalized === "skipped") return "warning";
  return "success";
}

export type JobStageTone = "success" | "active" | "idle";

export function jobStageTone(value: number): JobStageTone {
  if (value >= 100) return "success";
  if (value > 0) return "active";
  return "idle";
}

export function jobStageKey(label: string) {
  const key = label.toLowerCase();
  switch (key) {
    case "downloading":
      return {
        key: "downloading",
        color: "var(--accent)",
        soft: "rgba(214, 31, 52, 0.16)",
      };
    case "metadata":
      return {
        key: "metadata",
        color: "var(--warning)",
        soft: "rgba(193, 161, 90, 0.16)",
      };
    case "lyrics":
      return {
        key: "lyrics",
        color: "var(--success)",
        soft: "rgba(90, 197, 138, 0.16)",
      };
    default:
      return {
        key: "default",
        color: "var(--text-2)",
        soft: "rgba(255, 255, 255, 0.08)",
      };
  }
}

export function jobProgressFromStatus(status: string) {
  switch (status) {
    case "completed":
      return 100;
    case "running":
      return 50;
    case "failed":
      return 100;
    case "stopped":
      return 0;
    case "queued":
    default:
      return 0;
  }
}

export function splitJobDetail(detail: string): [string, string] {
  const lines = detail
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
  if (lines.length === 0) {
    return ["", ""];
  }
  if (lines.length === 1) {
    return [lines[0]!, ""];
  }
  return [lines[0]!, lines.slice(1).join(" · ")];
}

export function jobCanStart(status: string) {
  return status === "stopped" || status === "failed";
}

export function jobCanStop(status: string) {
  return status === "queued" || status === "running";
}
