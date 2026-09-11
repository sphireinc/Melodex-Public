import type {
  AppState,
  Job,
  MediaInfo,
  PlaybackState,
  Playlist,
  SettingsInput,
  TrackRecord,
  TrackPreview,
  ToolReadiness,
  URLImportDuplicateInfo,
} from "../types";
import * as AppAPI from "../../bindings/melodex/app";
import type * as BackendModels from "../../bindings/melodex/models";
import { createLargeVisualFixtureState, createVisualFixtureState, visualFixtureVideo } from "./visualFixtures";

type BackendAPI = typeof AppAPI;
type BackendAppState = Awaited<ReturnType<BackendAPI["GetState"]>>;
type BackendAppStateWithReadiness = BackendAppState & {
  toolReadiness?: Record<string, Partial<ToolReadiness>>;
};

const fallbackState: AppState = {
  settings: {
    libraryRoot: "~/Music/Melodex Music",
    aiBaseUrl: "https://api.openai.com/v1",
    aiModel: "gpt-4.1-mini",
    provider: "openai-compatible",
    apiKeyConfigured: false,
    updateManifestUrl: "",
    ytDlpPath: "",
    ffmpegPath: "",
    ytDlpCookiesPath: "",
    ytDlpCookiesFromBrowser: "",
    videoDownloadMode: "on-demand",
    downloadMusicVideo: false,
    keepOriginalAudio: true,
    maxConcurrentDownloads: 2,
    maxConcurrentVideoDownloads: 1,
    maxConcurrentEnrichmentRequests: 2,
    maxConcurrentLyricsRequests: 2,
    throttleOnYtdlpBotErrors: true,
  },
  stats: {
    trackCount: 0,
    artistCount: 0,
    albumCount: 0,
    jobCount: 0,
    pendingJobs: 0,
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
    appVersion: "1.0.0",
    buildNumber: "dev",
    gitCommit: "dev",
    buildTime: "unknown",
    releaseChannel: "dev",
    goVersion: "",
    settingsSchemaVersion: 2,
    catalogSchemaVersion: 1,
    libraryCacheSchemaVersion: 1,
    playlistSchemaVersion: 1,
    importHistorySchemaVersion: 1,
    trackMetadataSchemaVersion: 1,
  },
  updateInfo: {
    manifestUrl: "",
    currentVersion: "1.0.0",
    latestVersion: "",
    releaseNotes: "",
    downloadUrl: "",
    platform: "",
    available: false,
    mandatory: false,
    minimumVersion: "",
    status: "Update manifest not configured",
    error: "",
  },
  diagnostics: {
    logPath: "",
    lastExportPath: "",
    bundleStatus: "",
    bundleMessage: "",
  },
  aiStatus: "Missing API key",
  jobs: [],
  importHistory: [],
  libraryTracks: [],
  recentTracks: [],
  playlists: [],
  playback: {
    currentTrackId: "",
    currentTrackPath: "",
    queue: [],
    queueSource: "",
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
  toolReadiness: {
    ytDlp: {
      tool: "yt-dlp",
      status: "missing",
      error: "executable not found",
      remediation:
        "Install yt-dlp and ensure it is executable, or set its full executable path in Settings; then run the yt-dlp test again.",
    },
    ffmpeg: {
      tool: "ffmpeg",
      status: "missing",
      error: "executable not found",
      remediation:
        "Install ffmpeg and ensure it is executable, or set its full executable path in Settings; then run the ffmpeg test again.",
    },
  },
  rootInfo: {
    libraryRoot: "~/Music/Melodex Music",
    appDataDir: "",
    incomingDir: "",
    libraryDir: "",
    cacheDir: "",
  },
  windowFullscreen: false,
};

function visualFixtureEnabled() {
  return (
    import.meta.env.DEV &&
    typeof window !== "undefined" &&
    new URLSearchParams(window.location.search).get("visual") === "1"
  );
}

function largeVisualFixtureEnabled() {
  return visualFixtureEnabled() && new URLSearchParams(window.location.search).get("large") === "1";
}

function visualState() {
  const state = createVisualFixtureState(fallbackState);
  return largeVisualFixtureEnabled() ? createLargeVisualFixtureState(state) : state;
}

function backend() {
  if (typeof window === "undefined") return undefined;
  const wailsWindow = window as typeof window & { _wails?: { environment?: unknown } };
  if (!wailsWindow._wails?.environment) return undefined;
  return AppAPI as BackendAPI;
}

function isConnected() {
  return Boolean(backend());
}

function normalizeTrackRecord(track: BackendModels.TrackRecord): TrackRecord {
  return {
    ...track,
    videoPath: track.videoPath || undefined,
    artistTrivia: track.artistTrivia ?? undefined,
    albumTrivia: track.albumTrivia ?? undefined,
    songTrivia: track.songTrivia ?? undefined,
    tidbits: track.tidbits ?? undefined,
    sources: track.sources ?? undefined,
  };
}

function normalizePlaybackState(playback: BackendModels.PlaybackState): PlaybackState {
  return {
    ...playback,
    queue: playback.queue ?? [],
  };
}

function normalizePlaylist(playlist: BackendModels.Playlist): Playlist {
  return {
    ...playlist,
    trackIds: playlist.trackIds ?? [],
  };
}

function normalizeToolReadiness(value: unknown): Record<string, ToolReadiness> {
  const fallbackReadiness = fallbackState.toolReadiness ?? {};
  if (!value || typeof value !== "object") return fallbackReadiness;
  const entries = Object.entries(value as Record<string, Partial<ToolReadiness>>);
  if (entries.length === 0) return fallbackReadiness;
  return Object.fromEntries(
    entries.map(([key, result]) => [
      key,
      {
        tool: result.tool ?? key,
        status: result.status ?? "unchecked",
        version: result.version ?? "",
        error: result.error ?? "",
        remediation: result.remediation ?? "",
        correlationId: result.correlationId ?? "",
      },
    ]),
  );
}

function normalizeTrackPreview(preview: BackendModels.TrackPreview): TrackPreview {
  return {
    ...preview,
    track: normalizeTrackRecord(preview.track),
    fileStates: preview.fileStates ?? [],
    artistTrivia: preview.artistTrivia ?? undefined,
    albumTrivia: preview.albumTrivia ?? undefined,
    songTrivia: preview.songTrivia ?? undefined,
    tidbits: preview.tidbits ?? undefined,
    sources: preview.sources ?? undefined,
  };
}

function normalizeBackendState(state: BackendAppState): AppState {
  const stateWithReadiness = state as BackendAppStateWithReadiness;
  const settings: AppState["settings"] = {
    ...fallbackState.settings,
    ...state.settings,
    updateManifestUrl: state.settings.updateManifestUrl ?? "",
  };
  const updateInfo: AppState["updateInfo"] = {
    ...fallbackState.updateInfo,
    ...state.updateInfo,
    manifestUrl: state.updateInfo.manifestUrl ?? "",
    currentVersion: state.updateInfo.currentVersion ?? fallbackState.updateInfo.currentVersion,
    latestVersion: state.updateInfo.latestVersion ?? "",
    releaseNotes: state.updateInfo.releaseNotes ?? "",
    downloadUrl: state.updateInfo.downloadUrl ?? "",
    platform: state.updateInfo.platform ?? "",
    available: Boolean(state.updateInfo.available),
    mandatory: Boolean(state.updateInfo.mandatory),
    minimumVersion: state.updateInfo.minimumVersion ?? "",
    status: state.updateInfo.status ?? fallbackState.updateInfo.status,
    error: state.updateInfo.error ?? "",
  };

  return {
    settings,
    stats: {
      ...fallbackState.stats,
      ...state.stats,
    },
    health: {
      ...fallbackState.health,
      ...state.health,
    },
    workers: {
      ...fallbackState.workers,
      ...state.workers,
    },
    genreBuckets: Array.isArray(state.genreBuckets) ? state.genreBuckets : [],
    yearBuckets: Array.isArray(state.yearBuckets) ? state.yearBuckets : [],
    buildInfo: {
      ...fallbackState.buildInfo,
      ...state.buildInfo,
    },
    updateInfo,
    diagnostics: {
      ...fallbackState.diagnostics,
      ...state.diagnostics,
    },
    aiStatus: state.aiStatus ?? fallbackState.aiStatus,
    jobs: Array.isArray(state.jobs) ? state.jobs : [],
    importHistory: Array.isArray(state.importHistory) ? state.importHistory : [],
    libraryTracks: Array.isArray(state.libraryTracks) ? state.libraryTracks.map(normalizeTrackRecord) : [],
    recentTracks: Array.isArray(state.recentTracks) ? state.recentTracks.map(normalizeTrackRecord) : [],
    playlists: Array.isArray(state.playlists) ? state.playlists.map(normalizePlaylist) : [],
    playback: normalizePlaybackState({
      ...fallbackState.playback,
      ...state.playback,
      queue: state.playback.queue ?? [],
    }),
    promptFiles: Array.isArray(state.promptFiles) ? state.promptFiles : [],
    toolStatus: {
      ...fallbackState.toolStatus,
      ...state.toolStatus,
    },
    toolReadiness: normalizeToolReadiness(stateWithReadiness.toolReadiness),
    rootInfo: {
      ...fallbackState.rootInfo,
      ...state.rootInfo,
    },
    windowFullscreen: Boolean(state.windowFullscreen),
  };
}

export async function getState(): Promise<AppState> {
  const api = backend();
  if (!api) return visualFixtureEnabled() ? visualState() : fallbackState;
  return normalizeBackendState(await api.GetState());
}

export async function pickLocalFiles(): Promise<string[]> {
  const api = backend();
  if (!api) return [];
  return (await api.PickLocalFiles()) ?? [];
}

export async function pickLocalFolder(): Promise<string> {
  const api = backend();
  if (!api) return "";
  return api.PickLocalFolder();
}

export async function readFileDataURL(path: string): Promise<string> {
  const api = backend();
  if (!api) return "";
  return api.ReadFileDataURL(path);
}

export async function mediaURL(path: string): Promise<string> {
  const api = backend();
  if (!api && visualFixtureEnabled() && path.startsWith("fixture://")) return visualFixtureVideo.url;
  if (!api) return "";
  return (await api.MediaURLInfo(path)).url;
}

export async function mediaURLInfo(path: string): Promise<MediaInfo> {
  const api = backend();
  if (!api && visualFixtureEnabled() && path.startsWith("fixture://")) {
    return { url: visualFixtureVideo.url, mimeType: visualFixtureVideo.mimeType, reachable: true, statusCode: 200 };
  }
  if (!api) return { url: "", mimeType: "" };
  return api.MediaURLInfo(path);
}

export async function mediaInfo(path: string): Promise<MediaInfo> {
  const api = backend();
  if (!api && visualFixtureEnabled() && path.startsWith("fixture://")) {
    return { url: visualFixtureVideo.url, mimeType: visualFixtureVideo.mimeType, reachable: true, statusCode: 200 };
  }
  if (!api) return { url: "", mimeType: "" };
  return api.MediaInfo(path);
}

export async function logUIEvent(event: string, name: string, from: string): Promise<void> {
  const api = backend();
  if (!api) return;
  return api.LogUIEvent(event, name, from);
}

export async function logVideoEvent(event: string, source: string, detail: string): Promise<void> {
  const api = backend();
  if (!api) return;
  return api.LogVideoEvent(event, source, detail);
}

export async function pickCookiesFile(): Promise<string> {
  const api = backend();
  if (!api) return "";
  return api.PickCookiesFile();
}

export async function pickArtworkFile(): Promise<string> {
  const api = backend();
  if (!api) return "";
  return api.PickArtworkFile();
}

export async function toggleWindowMaximize(): Promise<void> {
  const api = backend();
  if (!api) return;
  return api.ToggleWindowMaximise();
}

export async function pickLibraryRoot(): Promise<string> {
  const api = backend();
  if (!api) return fallbackState.settings.libraryRoot;
  return api.PickLibraryRoot();
}

export async function saveSettings(input: SettingsInput): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.SaveSettings(input));
}

export async function resetSettings(): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.ResetSettings());
}

export async function clearAPIKey(): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.ClearAPIKey());
}

export async function testYTDLP(): Promise<string> {
  const api = backend();
  if (!api) return "yt-dlp unavailable";
  return api.TestYTDLP();
}

export async function testFFmpeg(): Promise<string> {
  const api = backend();
  if (!api) return "ffmpeg unavailable";
  return api.TestFFmpeg();
}

export async function testAI(): Promise<string> {
  const api = backend();
  if (!api) return "AI unavailable";
  return api.TestAI();
}

export async function clearImportHistory(): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.ClearImportHistory());
}

export async function applyArtworkFromFile(trackID: string, sourcePath: string): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.ApplyArtworkFromFile(trackID, sourcePath));
}

export async function repairLibraryFiles(): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.RepairLibraryFiles());
}

export async function reprocessUnprocessedTracks(): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.ReprocessUnprocessedTracks());
}

export async function reprocessTrackWithContext(trackID: string, userContext: string): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.ReprocessTrackWithContext(trackID, userContext));
}

export async function startJob(jobID: string): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.StartJob(jobID));
}

export async function stopJob(jobID: string): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.StopJob(jobID));
}

export async function pauseAllJobs(): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.PauseAllJobs());
}

export async function pausePlayback(): Promise<PlaybackState> {
  const api = backend();
  if (!api) return fallbackState.playback;
  return normalizePlaybackState(await api.PausePlayback());
}

export async function deleteAllJobs(): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.DeleteAllJobs());
}

export async function deleteDoneJobs(): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.DeleteDoneJobs());
}

export async function deleteQueuedJobs(): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.DeleteQueuedJobs());
}

export async function retryFailedPlaylistItems(jobID: string): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.RetryFailedPlaylistItems(jobID));
}

export async function downloadTrackVideo(trackID: string): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.DownloadTrackVideo(trackID));
}

export async function deleteJob(jobID: string): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.DeleteJob(jobID));
}

export async function checkForUpdates(): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.CheckForUpdates());
}

export async function exportDiagnostics(): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.ExportDiagnostics());
}

export async function openURL(url: string): Promise<void> {
  const api = backend();
  if (!api) return;
  return api.OpenURL(url);
}

export async function queueLocalFiles(paths: string[]): Promise<Job[]> {
  const api = backend();
  if (!api) return [];
  return (await api.QueueLocalFiles(paths)) ?? [];
}

export async function queueURLImport(url: string): Promise<Job> {
  const api = backend();
  if (!api) throw new Error("Melodex backend is not connected");
  return api.QueueURLImport(url);
}

export async function findURLImportDuplicate(url: string): Promise<URLImportDuplicateInfo> {
  const api = backend();
  if (!api) throw new Error("Melodex backend is not connected");
  return api.FindURLImportDuplicate(url);
}

export async function rescanLibrary(): Promise<AppState> {
  const api = backend();
  if (!api) return fallbackState;
  return normalizeBackendState(await api.RescanLibrary());
}

export async function inspectTrack(metadataPath: string): Promise<TrackPreview> {
  const api = backend();
  if (!api) throw new Error("Melodex backend is not connected");
  return normalizeTrackPreview(await api.InspectTrack(metadataPath));
}

export async function revealPath(path: string): Promise<void> {
  const api = backend();
  if (!api) return;
  return api.RevealPath(path);
}

export async function getPlaylists(): Promise<Playlist[]> {
  const api = backend();
  if (!api) return visualFixtureEnabled() ? visualState().playlists : [];
  return (await api.GetPlaylists())?.map(normalizePlaylist) ?? [];
}

export async function createPlaylist(name: string, description: string): Promise<Playlist> {
  const api = backend();
  if (!api) throw new Error("Melodex backend is not connected");
  return normalizePlaylist(await api.CreatePlaylist(name, description));
}

export async function renamePlaylist(playlistID: string, name: string, description: string): Promise<Playlist> {
  const api = backend();
  if (!api) throw new Error("Melodex backend is not connected");
  return normalizePlaylist(await api.RenamePlaylist(playlistID, name, description));
}

export async function deletePlaylist(playlistID: string): Promise<void> {
  const api = backend();
  if (!api) return;
  return api.DeletePlaylist(playlistID);
}

export async function addTrackToPlaylist(playlistID: string, trackID: string): Promise<Playlist> {
  const api = backend();
  if (!api) throw new Error("Melodex backend is not connected");
  return normalizePlaylist(await api.AddTrackToPlaylist(playlistID, trackID));
}

export async function removeTrackFromPlaylist(playlistID: string, trackID: string): Promise<Playlist> {
  const api = backend();
  if (!api) throw new Error("Melodex backend is not connected");
  return normalizePlaylist(await api.RemoveTrackFromPlaylist(playlistID, trackID));
}

export async function movePlaylistTrack(playlistID: string, trackID: string, delta: number): Promise<Playlist> {
  const api = backend();
  if (!api) throw new Error("Melodex backend is not connected");
  return normalizePlaylist(await api.MovePlaylistTrack(playlistID, trackID, delta));
}

export async function playTrack(trackID: string, queueTrackIDs: string[], queueSource: string): Promise<PlaybackState> {
  const api = backend();
  if (!api) {
    if (!visualFixtureEnabled()) return fallbackState.playback;
    const next = visualState().playback;
    return { ...next, currentTrackId: trackID, queue: queueTrackIDs, queueSource, isPlaying: true };
  }
  return normalizePlaybackState(await api.PlayTrack(trackID, queueTrackIDs, queueSource));
}

export async function togglePlayback(): Promise<PlaybackState> {
  const api = backend();
  if (!api)
    return visualFixtureEnabled()
      ? { ...visualState().playback, isPlaying: !visualState().playback.isPlaying }
      : fallbackState.playback;
  return normalizePlaybackState(await api.TogglePlayback());
}

export async function playNext(): Promise<PlaybackState> {
  const api = backend();
  if (!api) return fallbackState.playback;
  return normalizePlaybackState(await api.PlayNext());
}

export async function playPrevious(): Promise<PlaybackState> {
  const api = backend();
  if (!api) return fallbackState.playback;
  return normalizePlaybackState(await api.PlayPrevious());
}

export async function seekPlayback(ratio: number): Promise<PlaybackState> {
  const api = backend();
  if (!api) return fallbackState.playback;
  return normalizePlaybackState(await api.SeekPlayback(ratio));
}

export async function setPlaybackVolume(value: number): Promise<PlaybackState> {
  const api = backend();
  if (!api) return fallbackState.playback;
  return normalizePlaybackState(await api.SetPlaybackVolume(value));
}

export async function toggleMute(): Promise<PlaybackState> {
  const api = backend();
  if (!api) return fallbackState.playback;
  return normalizePlaybackState(await api.ToggleMute());
}

export async function toggleShuffle(): Promise<PlaybackState> {
  const api = backend();
  if (!api) return fallbackState.playback;
  return normalizePlaybackState(await api.ToggleShuffle());
}

export async function toggleRepeatMode(): Promise<PlaybackState> {
  const api = backend();
  if (!api) return fallbackState.playback;
  return normalizePlaybackState(await api.ToggleRepeatMode());
}

export async function clearPlaybackQueue(): Promise<PlaybackState> {
  const api = backend();
  if (!api) return fallbackState.playback;
  return normalizePlaybackState(await api.ClearPlaybackQueue());
}

export async function addTrackToPlaybackQueue(trackID: string): Promise<PlaybackState> {
  const api = backend();
  if (!api) return fallbackState.playback;
  return normalizePlaybackState(await api.AddTrackToPlaybackQueue(trackID));
}

export async function removeTrackFromPlaybackQueue(trackID: string): Promise<PlaybackState> {
  const api = backend();
  if (!api) return fallbackState.playback;
  return normalizePlaybackState(await api.RemoveTrackFromPlaybackQueue(trackID));
}

export async function shufflePlaybackQueue(): Promise<PlaybackState> {
  const api = backend();
  if (!api) return fallbackState.playback;
  return normalizePlaybackState(await api.ShufflePlaybackQueue());
}

export async function moveTrackInPlaybackQueue(trackID: string, delta: number): Promise<PlaybackState> {
  const api = backend();
  if (!api) return fallbackState.playback;
  return normalizePlaybackState(await api.MoveTrackInPlaybackQueue(trackID, delta));
}

export { isConnected };
