export type AppState = {
  settings: PublicSettings;
  stats: LibraryStats;
  health: LibraryHealth;
  workers: WorkerStats;
  genreBuckets: FacetBucket[];
  yearBuckets: FacetBucket[];
  buildInfo: BuildInfo;
  updateInfo: UpdateInfo;
  diagnostics: DiagnosticsInfo;
  aiStatus: string;
  jobs: Job[];
  importHistory: ImportHistoryEntry[];
  libraryTracks: TrackRecord[];
  recentTracks: TrackRecord[];
  playlists: Playlist[];
  playback: PlaybackState;
  promptFiles: PromptInfo[];
  toolStatus: ToolStatus;
  toolReadiness?: Record<string, ToolReadiness>;
  rootInfo: RootInfo;
  windowFullscreen: boolean;
};

export type ViewKey =
  | "library"
  | "index"
  | "artists"
  | "albums"
  | "songs"
  | "import"
  | "processing"
  | "playlists"
  | "settings";

export type LibraryMode = "albums" | "tracks" | "artists";

export type QueueSource = "library" | "playlist" | "search" | "album" | "manual";

export type RepeatMode = "off" | "all" | "one";

export type BrowseContext =
  | { mode: "library" }
  | {
      mode: "artist";
      artistKey: string;
      artistName: string;
    }
  | {
      mode: "album";
      artistKey: string;
      artistName: string;
      albumKey: string;
      albumTitle: string;
    }
  | {
      mode: "songs";
      artistKey?: string;
      artistName?: string;
      albumKey?: string;
      albumTitle?: string;
      filterLabel: string;
    };

export type PublicSettings = {
  libraryRoot: string;
  aiBaseUrl: string;
  aiModel: string;
  provider: string;
  apiKeyConfigured: boolean;
  updateManifestUrl?: string;
  ytDlpPath: string;
  ffmpegPath: string;
  ytDlpCookiesPath: string;
  ytDlpCookiesFromBrowser: string;
  videoDownloadMode: "off" | "on-demand" | "during-import" | string;
  downloadMusicVideo: boolean;
  keepOriginalAudio: boolean;
  maxConcurrentDownloads: number;
  maxConcurrentVideoDownloads: number;
  maxConcurrentEnrichmentRequests: number;
  maxConcurrentLyricsRequests: number;
  throttleOnYtdlpBotErrors: boolean;
};

export type SettingsInput = {
  libraryRoot: string;
  aiBaseUrl: string;
  aiModel: string;
  provider: string;
  apiKey: string;
  updateManifestUrl: string;
  ytDlpPath: string;
  ffmpegPath: string;
  ytDlpCookiesPath: string;
  ytDlpCookiesFromBrowser: string;
  videoDownloadMode: "off" | "on-demand" | "during-import" | string;
  downloadMusicVideo: boolean;
  keepOriginalAudio: boolean;
  maxConcurrentDownloads: number;
  maxConcurrentVideoDownloads: number;
  maxConcurrentEnrichmentRequests: number;
  maxConcurrentLyricsRequests: number;
  throttleOnYtdlpBotErrors: boolean;
};

export type ToolStatus = {
  ytDlp: boolean;
  ffmpeg: boolean;
  ai: boolean;
};

export type ToolReadiness = {
  tool: string;
  status: "ready" | "missing" | "probe-failed" | "timeout" | "unchecked" | string;
  version?: string;
  error?: string;
  remediation?: string;
  correlationId?: string;
};

export type BuildInfo = {
  appVersion: string;
  buildNumber: string;
  gitCommit: string;
  buildTime: string;
  releaseChannel: string;
  goVersion: string;
  settingsSchemaVersion: number;
  catalogSchemaVersion: number;
  libraryCacheSchemaVersion: number;
  playlistSchemaVersion: number;
  importHistorySchemaVersion: number;
  trackMetadataSchemaVersion: number;
};

export type UpdateInfo = {
  manifestUrl?: string;
  currentVersion: string;
  latestVersion: string;
  releaseDate?: string;
  releaseNotes: string;
  downloadUrl: string;
  platform: string;
  available: boolean;
  mandatory: boolean;
  minimumVersion: string;
  checkedAt?: string;
  status: string;
  error: string;
};

export type DiagnosticsInfo = {
  logPath: string;
  lastExportPath: string;
  lastExportedAt?: string;
  bundleStatus: string;
  bundleMessage: string;
};

export type LibraryStats = {
  trackCount: number;
  artistCount: number;
  albumCount: number;
  jobCount: number;
  pendingJobs: number;
  failedJobs: number;
};

export type LibraryHealth = {
  totalTracks: number;
  readyTracks: number;
  unprocessedTracks: number;
  missingAudio: number;
  missingLyrics: number;
  missingTimed: number;
  missingMetadata: number;
};

export type WorkerStats = {
  activeWorkers: number;
  maxWorkers: number;
  videoActive: number;
  videoQueued: number;
  audio?: WorkerClassStats;
  video?: WorkerClassStats;
  enrichment?: WorkerClassStats;
  lyrics?: WorkerClassStats;
  artwork?: WorkerClassStats;
  throttle?: WorkerThrottleState;
  blockedReason?: string;
};

export type WorkerClassStats = {
  capacity: number;
  active: number;
  queued: number;
  blocked: boolean;
  blockedReason?: string;
  sharedCapacityOf?: string;
};

export type WorkerThrottleState = {
  active: boolean;
  until?: string;
  reason?: string;
};

export type RootInfo = {
  libraryRoot: string;
  appDataDir: string;
  incomingDir: string;
  libraryDir: string;
  cacheDir: string;
};

export type MediaInfo = {
  url: string;
  mimeType: string;
  reachable?: boolean;
  statusCode?: number;
  contentLength?: number;
  expiresAt?: string;
};

export type PromptInfo = {
  name: string;
  purpose: string;
};

export type FacetBucket = {
  value: string;
  count: number;
};

export type ImportHistoryEntry = {
  url: string;
  completedAt: string;
};

export type URLImportDuplicateInfo = {
  exists: boolean;
  trackId?: string;
  jobId?: string;
  metadataPath?: string;
  title?: string;
  artist?: string;
  album?: string;
  sourceUrl?: string;
  videoUrl?: string;
  matchReason?: string;
};

export type JobStageStatuses = {
  download?: string;
  metadata?: string;
  musicBrainz?: string;
  lyrics?: string;
  artwork?: string;
  video?: string;
  finalize?: string;
};

export type JobProgressEvent = {
  jobId: string;
  downloadProgress: number;
  metadataProgress: number;
  lyricsProgress: number;
  status: string;
  detail?: string;
  stageStatuses?: JobStageStatuses;
  parentJobId?: string;
  playlistTotal?: number;
  playlistProcessed?: number;
  playlistFailed?: number;
};

export type Job = {
  id: string;
  kind: string;
  input: string;
  sourceUrl?: string;
  sourceRoot?: string;
  downloadVideo?: boolean;
  trackId?: string;
  userContext?: string;
  parentJobId?: string;
  playlistUrl?: string;
  playlistTitle?: string;
  playlistChannel?: string;
  playlistTotalItems?: number;
  playlistProcessedItems?: number;
  playlistFailedItems?: number;
  playlistIndex?: number;
  playlistCurrentIndex?: number;
  playlistCurrentTitle?: string;
  playlistItemTitle?: string;
  status: string;
  detail: string;
  error?: string;
  outputBundle?: string;
  stageStatuses?: JobStageStatuses;
  downloadProgress?: number;
  metadataProgress?: number;
  lyricsProgress?: number;
  resultTitle?: string;
  resultArtist?: string;
  resultAlbum?: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
};

export type TrackRecord = {
  id: string;
  artist: string;
  album: string;
  title: string;
  genre: string;
  year: string;
  hasLyrics?: boolean;
  needsReview?: boolean;
  duration?: number;
  lyricsIncluded: boolean;
  confidence?: string;
  bundlePath: string;
  storageDir: string;
  audioPath: string;
  lyricsPath: string;
  lrcPath: string;
  artworkPath: string;
  videoPath?: string;
  artworkDataUrl?: string;
  artworkMediaUrl?: string;
  artworkUrl?: string;
  videoUrl?: string;
  metadataPath: string;
  metadataConfidence?: string;
  enrichmentConfidence?: string;
  releaseType?: string;
  isrc?: string;
  artistLinks?: MetadataLinks;
  albumLinks?: MetadataLinks;
  songLinks?: MetadataLinks;
  artistTrivia?: string[];
  albumTrivia?: string[];
  songTrivia?: string[];
  songMeaning?: string;
  tidbits?: string[];
  sources?: string[];
  metadataNotes?: string;
  lyricsConfidence?: string;
  lyricsSource?: string;
  lyricsSourceLoc?: string;
  timedLyricsConfidence?: string;
  timedLyricsGranularity?: string;
  hasTimedLyrics: boolean;
  sourceTitle?: string;
  sourceChannel?: string;
  aiProvider?: string;
  aiModel?: string;
  aiRan?: boolean;
  aiStatus?: string;
  aiMessage?: string;
  aiContext?: AIContext;
  generatedAt?: string;
  sourceKind: string;
  sourceRef: string;
  durationSeconds?: number | null;
  metadataSize?: number;
  metadataModTime?: string;
  hash: string;
  createdAt: string;
};

export type Playlist = {
  id: string;
  name: string;
  description: string;
  createdAt: string;
  updatedAt: string;
  trackIds: string[];
};

export type TrackFileState = {
  label: string;
  path: string;
  exists: boolean;
};

export type MetadataLinks = {
  officialWebsite?: string;
  spotify?: string;
  appleMusic?: string;
  youtube?: string;
  youtubeMusic?: string;
  instagram?: string;
  x?: string;
  facebook?: string;
  bandcamp?: string;
  soundcloud?: string;
  wikipedia?: string;
  musicBrainz?: string;
  genius?: string;
};

export type TrackPreview = {
  track: TrackRecord;
  lyricsText: string;
  timedLyricsText: string;
  fileStates: TrackFileState[];
  title: string;
  artist: string;
  album: string;
  trackNumber?: number | null;
  year?: number | null;
  genre: string;
  durationSeconds?: number | null;
  confidence?: string;
  metadataConfidence?: string;
  enrichmentConfidence?: string;
  releaseType?: string;
  isrc?: string;
  artistLinks?: MetadataLinks;
  albumLinks?: MetadataLinks;
  songLinks?: MetadataLinks;
  artistTrivia?: string[];
  albumTrivia?: string[];
  songTrivia?: string[];
  songMeaning?: string;
  tidbits?: string[];
  sources?: string[];
  metadataNotes?: string;
  sourceUrl: string;
  sourceProvider: string;
  sourceVideoId: string;
  sourceTitle: string;
  sourceChannel: string;
  aiProvider: string;
  aiModel: string;
  aiRan: boolean;
  aiStatus?: string;
  aiMessage?: string;
  aiContext?: AIContext;
  generatedAt: string;
  lyricsConfidence?: string;
  lyricsSource?: string;
  lyricsSourceLoc?: string;
  timedLyricsConfidence?: string;
  timedLyricsSource?: string;
  timedLyricsGranularity?: string;
  artworkPath?: string;
  artworkDataUrl?: string;
  artworkMediaUrl?: string;
  videoPath?: string;
  videoUrl?: string;
};

export type PlaybackState = {
  currentTrackId: string;
  currentTrackPath: string;
  queue: string[];
  queueSource: string;
  isPlaying: boolean;
  isLoading: boolean;
  currentTime: number;
  duration: number;
  volume: number;
  muted: boolean;
  shuffleEnabled: boolean;
  repeatMode: RepeatMode | string;
  error: string;
};

export type AIContext = {
  file_name?: string;
  source_ref?: string;
  source_url?: string;
  source_kind?: string;
  source_title?: string;
  source_uploader?: string;
  source_channel?: string;
  library_root?: string;
  source_description_hint?: string;
  duration_seconds?: number | null;
  duration_hint?: string;
  has_transcript?: boolean;
  has_timestamped_transcript?: boolean;
  input_text?: string;
  user_context?: string;
  artist_hint?: string;
  album_hint?: string;
  title_hint?: string;
  year_hint?: number | null;
  genre_hint?: string;
  track_number_hint?: number | null;
  notes?: string;
  resolved_title?: string;
  resolved_artist?: string;
  resolved_album?: string;
  resolved_track_number?: string;
  resolved_year?: string;
  resolved_genre?: string;
  resolved_confidence?: string;
  resolved_notes?: string;
  target_artist_dir?: string;
  target_album_dir?: string;
  target_base_name?: string;
  target_audio_path?: string;
  target_lyrics_path?: string;
  target_timed_lyrics_path?: string;
  target_metadata_path?: string;
};

declare global {
  interface Window {
    go?: {
      main?: {
        App?: {
          GetState: () => Promise<AppState>;
          PickLocalFiles: () => Promise<string[]>;
          PickLibraryRoot: () => Promise<string>;
          SaveSettings: (input: SettingsInput) => Promise<AppState>;
          ClearAPIKey: () => Promise<AppState>;
          ClearImportHistory: () => Promise<AppState>;
          RepairLibraryFiles: () => Promise<AppState>;
          ReprocessUnprocessedTracks: () => Promise<AppState>;
          ReprocessTrackWithContext: (trackID: string, userContext: string) => Promise<AppState>;
          StartJob: (jobID: string) => Promise<AppState>;
          StopJob: (jobID: string) => Promise<AppState>;
          PauseAllJobs: () => Promise<AppState>;
          PausePlayback: () => Promise<PlaybackState>;
          DeleteAllJobs: () => Promise<AppState>;
          DeleteDoneJobs: () => Promise<AppState>;
          DeleteQueuedJobs: () => Promise<AppState>;
          RetryFailedPlaylistItems: (jobID: string) => Promise<AppState>;
          DeleteJob: (jobID: string) => Promise<AppState>;
          CheckForUpdates: () => Promise<AppState>;
          ExportDiagnostics: () => Promise<AppState>;
          OpenURL: (url: string) => Promise<void>;
          QueueLocalFiles: (paths: string[]) => Promise<Job[]>;
          QueueURLImport: (url: string) => Promise<Job>;
          RescanLibrary: () => Promise<AppState>;
          InspectTrack: (metadataPath: string) => Promise<TrackPreview>;
          RevealPath: (path: string) => Promise<void>;
          GetPlaylists: () => Promise<Playlist[]>;
          CreatePlaylist: (name: string, description: string) => Promise<Playlist>;
          RenamePlaylist: (playlistID: string, name: string, description: string) => Promise<Playlist>;
          DeletePlaylist: (playlistID: string) => Promise<void>;
          AddTrackToPlaylist: (playlistID: string, trackID: string) => Promise<Playlist>;
          RemoveTrackFromPlaylist: (playlistID: string, trackID: string) => Promise<Playlist>;
          MovePlaylistTrack: (playlistID: string, trackID: string, delta: number) => Promise<Playlist>;
          PlayTrack: (trackID: string, queueTrackIDs: string[], queueSource: string) => Promise<PlaybackState>;
          TogglePlayback: () => Promise<PlaybackState>;
          DownloadTrackVideo: (trackID: string) => Promise<AppState>;
          MediaURL: (path: string) => Promise<string>;
          MediaURLInfo: (path: string) => Promise<MediaInfo>;
          ResetSettings: () => Promise<AppState>;
          TestYTDLP: () => Promise<string>;
          TestFFmpeg: () => Promise<string>;
          TestAI: () => Promise<string>;
          PlayNext: () => Promise<PlaybackState>;
          PlayPrevious: () => Promise<PlaybackState>;
          SeekPlayback: (ratio: number) => Promise<PlaybackState>;
          SetPlaybackVolume: (value: number) => Promise<PlaybackState>;
          ToggleMute: () => Promise<PlaybackState>;
          ToggleShuffle: () => Promise<PlaybackState>;
          ToggleRepeatMode: () => Promise<PlaybackState>;
          ClearPlaybackQueue: () => Promise<PlaybackState>;
          AddTrackToPlaybackQueue: (trackID: string) => Promise<PlaybackState>;
          RemoveTrackFromPlaybackQueue: (trackID: string) => Promise<PlaybackState>;
          ShufflePlaybackQueue: () => Promise<PlaybackState>;
          MoveTrackInPlaybackQueue: (trackID: string, delta: number) => Promise<PlaybackState>;
        };
      };
    };
  }
}
