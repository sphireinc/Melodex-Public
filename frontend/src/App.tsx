import { useEffect, useMemo, useRef, useState } from "react";
import {
  addTrackToPlaylist,
  createPlaylist,
  clearAPIKey,
  clearImportHistory,
  checkForUpdates,
  clearPlaybackQueue,
  deleteAllJobs,
  deletePlaylist,
  deleteDoneJobs,
  deleteQueuedJobs,
  inspectTrack,
  pickLibraryRoot,
  pickArtworkFile,
  pickLocalFiles,
  pickLocalFolder,
  resetSettings,
  startJob,
  playNext as backendPlayNext,
  playPrevious as backendPlayPrevious,
  playTrack as backendPlayTrack,
  seekPlayback as backendSeekPlayback,
  pauseAllJobs,
  applyArtworkFromFile,
  movePlaylistTrack,
  moveTrackInPlaybackQueue,
  removeTrackFromPlaylist,
  removeTrackFromPlaybackQueue,
  retryFailedPlaylistItems,
  reprocessTrackWithContext,
  queueLocalFiles,
  queueURLImport,
  exportDiagnostics,
  repairLibraryFiles,
  renamePlaylist,
  findURLImportDuplicate,
  openURL,
  revealPath,
  rescanLibrary,
  reprocessUnprocessedTracks,
  saveSettings,
  logUIEvent,
  testAI,
  testFFmpeg,
  testYTDLP,
  setPlaybackVolume as backendSetPlaybackVolume,
  logVideoEvent,
  stopJob,
  deleteJob,
  shufflePlaybackQueue,
  toggleMute as backendToggleMute,
  togglePlayback as backendTogglePlayback,
  toggleRepeatMode as backendToggleRepeatMode,
  toggleShuffle as backendToggleShuffle,
  toggleWindowMaximize,
} from "./lib/backend";
import { CompactPlayerBar } from "./components/Player/CompactPlayerBar";
import { LyricsSidePanel } from "./components/Player/LyricsSidePanel";
import { PlaybackQueueDrawer } from "./components/Queue/PlaybackQueueDrawer";
import { PlaybackTime } from "./components/Queue/PlaybackTime";
import { ImmersivePlayerView } from "./components/Player/ImmersivePlayerView";
import { KeyValue } from "./components/Common";
import {
  ModernSidebar,
  MusicBrowserPage,
  LibraryIndexPage,
  ImportPageModern,
  ProcessingPageModern,
  SettingsPageModern,
  PlaylistsPageModern,
} from "./components/ModernViews";
import { PlaylistCreateModal, AddToPlaylistModal } from "./components/Playlists/PlaylistModals";
import { useKeyboardShortcuts } from "./hooks/useKeyboardShortcuts";
import { useMelodexEvents } from "./hooks/useMelodexEvents";
import { useMelodexAppState } from "./hooks/useMelodexAppState";
import { useTooltip } from "./hooks/useTooltip";
import { useVideoPlayback } from "./hooks/useVideoPlayback";
import {
  currentTracks,
  filterTracks,
  pageSubtitle,
  pageTitle,
  getTracksForBrowseContext,
  splitImportUrls,
} from "./lib/viewHelpers";
import {
  serializeVideoDiagnosticContext,
  videoSourceType as videoDiagnosticSourceType,
} from "./lib/videoDiagnostics";
import { TrackProfileModal } from "./components/TrackProfile/TrackProfileModal";
import type {
  AppState,
  Job,
  PlaybackState,
  RepeatMode,
  SettingsInput,
  URLImportDuplicateInfo,
  TrackPreview,
  TrackRecord,
} from "./types";

type ViewKey =
  | "library"
  | "index"
  | "artists"
  | "albums"
  | "songs"
  | "import"
  | "processing"
  | "playlists"
  | "settings";
type QueueSource = "library" | "playlist" | "search" | "album" | "manual";
type BrowseContext =
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
type BrowseHistoryEntry = {
  context: BrowseContext;
  view: ViewKey;
};
const emptySettings: SettingsInput = {
  libraryRoot: "",
  aiBaseUrl: "",
  aiModel: "",
  provider: "",
  apiKey: "",
  updateManifestUrl: "",
  ytDlpPath: "",
  ffmpegPath: "",
  ytDlpCookiesPath: "",
  ytDlpCookiesFromBrowser: "",
  videoDownloadMode: "on-demand",
  downloadMusicVideo: false,
  keepOriginalAudio: false,
  maxConcurrentDownloads: 2,
  maxConcurrentVideoDownloads: 1,
  maxConcurrentEnrichmentRequests: 2,
  maxConcurrentLyricsRequests: 2,
  throttleOnYtdlpBotErrors: true,
};

const emptyAppState: AppState = {
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
    keepOriginalAudio: false,
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
    releaseDate: undefined,
    releaseNotes: "",
    downloadUrl: "",
    platform: "",
    available: false,
    mandatory: false,
    minimumVersion: "",
    checkedAt: undefined,
    status: "Update manifest not configured",
    error: "",
  },
  diagnostics: {
    logPath: "",
    lastExportPath: "",
    lastExportedAt: undefined,
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
    libraryRoot: "~/Music/Melodex Music",
    appDataDir: "",
    incomingDir: "",
    libraryDir: "",
    cacheDir: "",
  },
  windowFullscreen: false,
};

function normalizeAppState(input?: Partial<AppState> | null): AppState {
  const settings = input?.settings ?? emptyAppState.settings;
  const stats = input?.stats ?? emptyAppState.stats;
  const health = input?.health ?? emptyAppState.health;
  const workers = input?.workers ?? emptyAppState.workers;
  const buildInfo = input?.buildInfo ?? emptyAppState.buildInfo;
  const updateInfo = input?.updateInfo ?? emptyAppState.updateInfo;
  const diagnostics = input?.diagnostics ?? emptyAppState.diagnostics;
  const playback = input?.playback ?? emptyAppState.playback;
  const toolStatus = input?.toolStatus ?? emptyAppState.toolStatus;
  const rootInfo = input?.rootInfo ?? emptyAppState.rootInfo;

  return {
    settings: {
      ...emptyAppState.settings,
      ...settings,
    },
    stats: {
      ...emptyAppState.stats,
      ...stats,
    },
    health: {
      ...emptyAppState.health,
      ...health,
    },
    workers: {
      ...emptyAppState.workers,
      ...workers,
    },
    genreBuckets: Array.isArray(input?.genreBuckets) ? input.genreBuckets : [],
    yearBuckets: Array.isArray(input?.yearBuckets) ? input.yearBuckets : [],
    buildInfo: {
      ...emptyAppState.buildInfo,
      ...buildInfo,
    },
    updateInfo: {
      ...emptyAppState.updateInfo,
      ...updateInfo,
    },
    diagnostics: {
      ...emptyAppState.diagnostics,
      ...diagnostics,
    },
    aiStatus: input?.aiStatus ?? emptyAppState.aiStatus,
    jobs: Array.isArray(input?.jobs) ? input.jobs : [],
    importHistory: Array.isArray(input?.importHistory) ? input.importHistory : [],
    libraryTracks: Array.isArray(input?.libraryTracks) ? input.libraryTracks : [],
    recentTracks: Array.isArray(input?.recentTracks) ? input.recentTracks : [],
    playlists: Array.isArray(input?.playlists) ? input.playlists : [],
    playback: {
      ...emptyAppState.playback,
      ...playback,
      queue: Array.isArray(playback?.queue) ? playback.queue : [],
    },
    promptFiles: Array.isArray(input?.promptFiles) ? input.promptFiles : [],
    toolStatus: {
      ...emptyAppState.toolStatus,
      ...toolStatus,
    },
    rootInfo: {
      ...emptyAppState.rootInfo,
      ...rootInfo,
    },
    windowFullscreen: Boolean(input?.windowFullscreen),
  };
}

function normalizeRepeatMode(value: string): RepeatMode {
  if (value === "all" || value === "one") {
    return value;
  }
  return "off";
}

function mimeTypeFromPath(path: string) {
  const lower = path.trim().toLowerCase();
  if (lower.endsWith(".webm")) return "video/webm";
  if (lower.endsWith(".mov")) return "video/quicktime";
  if (lower.endsWith(".m4v")) return "video/x-m4v";
  if (lower.endsWith(".ogv") || lower.endsWith(".ogg")) return "video/ogg";
  return "video/mp4";
}

export default function App() {
  const [view, setView] = useState<ViewKey>("library");
  const [search, setSearch] = useState("");
  const [filterLyrics, setFilterLyrics] = useState(false);
  const [filterNeedsReview, setFilterNeedsReview] = useState(false);
  const [filterMissingMetadata, setFilterMissingMetadata] = useState(false);
  const [selectedGenre, setSelectedGenre] = useState("");
  const [selectedYear, setSelectedYear] = useState("");
  const [selectedTrackPath, setSelectedTrackPath] = useState("");
  const [browseContext, setBrowseContext] = useState<BrowseContext>({ mode: "library" });
  const [browseHistory, setBrowseHistory] = useState<BrowseHistoryEntry[]>([]);
  const [selectedJobId, setSelectedJobId] = useState("");
  const [selectedPlaylistId, setSelectedPlaylistId] = useState("");
  const [preview, setPreview] = useState<TrackPreview | null>(null);
  const [previewStatus, setPreviewStatus] = useState("Loading preview");
  const [previewLoading, setPreviewLoading] = useState(false);
  const [url, setUrl] = useState("");
  const [pendingDuplicateImport, setPendingDuplicateImport] = useState<{
    url: string;
    info: URLImportDuplicateInfo;
  } | null>(null);
  const [apiKeyDraft, setApiKeyDraft] = useState("");
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [lyricsOpen, setLyricsOpen] = useState(false);
  const [playerOpen, setPlayerOpen] = useState(false);
  const [queueOpen, setQueueOpen] = useState(false);
  const [playlistCreateOpen, setPlaylistCreateOpen] = useState(false);
  const [playlistCreateName, setPlaylistCreateName] = useState("");
  const [playlistCreateDescription, setPlaylistCreateDescription] = useState("");
  const [playlistEditOpen, setPlaylistEditOpen] = useState(false);
  const [playlistEditId, setPlaylistEditId] = useState("");
  const [playlistEditName, setPlaylistEditName] = useState("");
  const [playlistEditDescription, setPlaylistEditDescription] = useState("");
  const [playlistModalOpen, setPlaylistModalOpen] = useState(false);
  const [playlistDraftName, setPlaylistDraftName] = useState("");
  const [playlistDraftDescription, setPlaylistDraftDescription] = useState("");
  const [playlistTrackTarget, setPlaylistTrackTarget] = useState<TrackRecord | null>(null);
  const [playerInfoOpen, setPlayerInfoOpen] = useState(false);
  const [addContextOpen, setAddContextOpen] = useState(false);
  const [addContextDraft, setAddContextDraft] = useState("");
  const [settingsSaveStatus, setSettingsSaveStatus] = useState("");
  const [settingsSaveScope, setSettingsSaveScope] = useState("");
  const [seekPreviewRatio, setSeekPreviewRatio] = useState<number | null>(null);
  const liveAudioTimeRef = useRef(0);
  const [player, setPlayer] = useState<PlaybackState>({
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
  });
  const seekDebounceRef = useRef<number | null>(null);
  const seekRequestSeqRef = useRef(0);
  const seekActiveTrackRef = useRef("");
  const settingsSaveTimerRef = useRef<number | null>(null);
  const { hoverTooltip, tooltipRef, handleTooltipMove, handleTooltipLeave } = useTooltip();
  const {
    state,
    setState,
    settings,
    setSettings,
    status,
    setStatus,
    busy,
    setBusy,
    hasLoadedRef,
    syncSettingsFromAppState,
    refresh,
  } = useMelodexAppState({
    initialSettings: emptySettings,
    normalizeAppState,
    setPlayer,
  });
  const appState = useMemo(() => normalizeAppState(state), [state]);
  const tracks = useMemo(() => currentTracks(appState), [appState]);
  const tracksByID = useMemo(() => new Map(tracks.map((track) => [track.id, track])), [tracks]);
  const trackById = (trackID: string) => tracksByID.get(trackID) ?? null;
  const {
    videoOpen,
    videoFullscreenOpen,
    videoSourceUrl,
    videoSourceType: videoSourceTypeFromServer,
    videoCurrentTime,
    videoDuration,
    videoIsPlaying,
    videoShouldPlay,
    videoLoading,
    videoError,
    videoMuted,
    videoVolume,
    videoTrackAvailable,
    videoPlayerRef,
    openVideoForTrack,
    closeVideo,
    handleVideoSeek,
    handleVideoPlayPause,
    handleVideoVolume,
    handleVideoToggleMute,
    setVideoCurrentTime,
    setVideoDuration,
    setVideoLoading,
    setVideoError,
    setVideoShouldPlay,
    setVideoIsPlaying,
  } = useVideoPlayback({
    player,
    downloadMode: settings.videoDownloadMode,
    setPlayer,
    setState: (next) => {
      setState(next);
    },
    trackById: (trackID) => trackById(trackID),
  });
  useEffect(() => {
    return () => {
      if (settingsSaveTimerRef.current !== null) {
        window.clearTimeout(settingsSaveTimerRef.current);
      }
    };
  }, []);

  useEffect(() => {
    const handleDocumentClick = (event: MouseEvent) => {
      const target = event.target as HTMLElement | null;
      const actionable = target?.closest("button, [role='button']") as HTMLElement | null;
      if (!actionable) return;

      if (actionable.matches("button:disabled") || actionable.getAttribute("aria-disabled") === "true") {
        return;
      }

      const explicitLabel =
        actionable.getAttribute("data-tooltip") ??
        actionable.getAttribute("aria-label") ??
        actionable.getAttribute("title") ??
        "";
      const label = (explicitLabel || (actionable.textContent ?? "")).replace(/\s+/g, " ").trim();
      if (!label || (label.length <= 2 && !/[A-Za-z0-9]/.test(label))) {
        return;
      }

      const kind = actionable.closest("nav.sidebar, .sidebar, .modern-sidebar") ? "menu_click" : "button_click";
      void logUIEvent(kind, label, pageTitle(view)).catch(() => {});
    };

    document.addEventListener("click", handleDocumentClick, true);
    return () => document.removeEventListener("click", handleDocumentClick, true);
  }, [view]);

  useMelodexEvents({
    refresh,
    setState,
    setPlayer,
    syncSettingsFromAppState,
    hasLoadedRef,
    normalizeAppState,
    setStatus,
  });

  useEffect(() => {
    if (tracks.length === 0) {
      setSelectedTrackPath("");
      setPreview(null);
      return;
    }
    if (!selectedTrackPath) {
      setSelectedTrackPath(tracks[0]!.metadataPath);
      return;
    }
    if (!tracks.some((track) => track.metadataPath === selectedTrackPath)) {
      setSelectedTrackPath(tracks[0]!.metadataPath);
    }
  }, [tracks, selectedTrackPath]);

  useEffect(() => {
    if (browseContext.mode === "library") {
      return;
    }
    const filteredBase = filterTracks(tracks, search, {
      lyrics: filterLyrics,
      needsReview: filterNeedsReview,
      missingMetadata: filterMissingMetadata,
      genre: selectedGenre,
      year: selectedYear,
    });
    const nextTracks = getTracksForBrowseContext(filteredBase, browseContext);
    if (nextTracks.length > 0) {
      return;
    }
    setBrowseContext({ mode: "library" });
    setBrowseHistory([]);
  }, [
    browseContext,
    tracks,
    search,
    filterLyrics,
    filterNeedsReview,
    filterMissingMetadata,
    selectedGenre,
    selectedYear,
  ]);

  const selectedTrack = tracks.find((track) => track.metadataPath === selectedTrackPath) ?? null;
  const currentTrack = trackById(player.currentTrackId);
  const previewTargetPath = currentTrack?.metadataPath ?? selectedTrack?.metadataPath ?? "";

  useEffect(() => {
    if (!previewTargetPath) {
      setPreview(null);
      setPreviewStatus("No track selected");
      return;
    }
    let active = true;
    setPreviewLoading(true);
    setPreviewStatus("Loading preview");
    void inspectTrack(previewTargetPath)
      .then((next) => {
        if (!active) return;
        setPreview(next);
        setPreviewStatus("Preview ready");
      })
      .catch((error) => {
        if (!active) return;
        setPreview(null);
        setPreviewStatus(error instanceof Error ? error.message : String(error));
      })
      .finally(() => {
        if (active) setPreviewLoading(false);
      });
    return () => {
      active = false;
    };
  }, [previewTargetPath]);

  useEffect(() => {
    const jobs = appState.jobs;
    if (view === "processing" && jobs.length > 0 && !selectedJobId) {
      setSelectedJobId(jobs[0]!.id);
    }
    if (selectedJobId && !jobs.some((job) => job.id === selectedJobId)) {
      setSelectedJobId(jobs[0]?.id ?? "");
    }
  }, [appState.jobs, selectedJobId, view]);

  useEffect(() => {
    if (!appState.playlists.length) return;
    if (!selectedPlaylistId) {
      setSelectedPlaylistId(appState.playlists[0]!.id);
      return;
    }
    if (!appState.playlists.some((playlist) => playlist.id === selectedPlaylistId)) {
      setSelectedPlaylistId(appState.playlists[0]!.id);
    }
  }, [appState.playlists, selectedPlaylistId]);

  useEffect(() => {
    if (!selectedGenre) return;
    const nextGenre = selectedGenre.trim().toLowerCase();
    if (!appState.genreBuckets.some((bucket) => bucket.value.trim().toLowerCase() === nextGenre)) {
      setSelectedGenre("");
    }
  }, [appState.genreBuckets, selectedGenre]);

  useEffect(() => {
    if (!selectedYear) return;
    const nextYear = selectedYear.trim();
    if (!appState.yearBuckets.some((bucket) => bucket.value.trim() === nextYear)) {
      setSelectedYear("");
    }
  }, [appState.yearBuckets, selectedYear]);

  useEffect(() => {
    if (player.currentTrackId === seekActiveTrackRef.current) return;
    seekActiveTrackRef.current = player.currentTrackId;
    setSeekPreviewRatio(null);
    if (seekDebounceRef.current !== null) {
      window.clearTimeout(seekDebounceRef.current);
      seekDebounceRef.current = null;
    }
    seekRequestSeqRef.current += 1;
  }, [player.currentTrackId]);

  useKeyboardShortcuts({
    currentTrackId: player.currentTrackId,
    togglePlayback,
    playPrevious,
    playNext,
    seekBySeconds,
    toggleMute,
    setQueueOpen,
    setLyricsOpen,
  });

  async function withBusy<T>(label: string, fn: () => Promise<T>) {
    setBusy(true);
    setStatus(label);
    try {
      const result = await fn();
      await refresh();
      setStatus("Ready");
      return result;
    } catch (error) {
      setStatus(error instanceof Error ? error.message : String(error));
      return undefined as T;
    } finally {
      setBusy(false);
    }
  }

  async function handleChooseRoot() {
    const picked = await withBusy("Choosing library root", pickLibraryRoot);
    if (picked) {
      setSettings((current) => ({ ...current, libraryRoot: picked }));
    }
  }

  async function handleChooseFiles() {
    const paths = await withBusy("Choosing local files", pickLocalFiles);
    if (paths.length > 0) {
      await withBusy("Queueing local files", () => queueLocalFiles(paths));
    }
  }

  async function handleChooseFolder() {
    const folder = await withBusy("Choosing local folder", pickLocalFolder);
    if (folder) {
      await withBusy("Queueing local folder", () => queueLocalFiles([folder]));
    }
  }

  async function handleToggleWindowMaximize() {
    await toggleWindowMaximize();
    await refresh();
  }

  async function handleQueueURL() {
    const links = splitImportUrls(url);
    if (links.length === 0) return;
    if (links.length === 1) {
      const firstLink = links[0]!;
      const duplicate = await findURLImportDuplicate(firstLink);
      if (duplicate.exists) {
        setPendingDuplicateImport({ url: firstLink, info: duplicate });
        setStatus("Duplicate URL detected");
        return;
      }
    }
    await withBusy(`Queueing ${links.length} URL${links.length === 1 ? "" : "s"}`, async () => {
      for (const link of links) {
        await queueURLImport(link);
      }
    });
    setUrl("");
  }

  async function handleClearImportHistory() {
    await withBusy("Clearing import history", clearImportHistory);
  }

  async function handleDuplicateOpenExisting() {
    const duplicate = pendingDuplicateImport?.info;
    if (!duplicate) return;
    const track = tracks.find((entry) => entry.id === duplicate.trackId) ?? null;
    setPendingDuplicateImport(null);
    if (!track) {
      setStatus("Duplicate track not found");
      return;
    }
    setSelectedTrackPath(track.metadataPath);
    setView("songs");
    setBrowseContext({ mode: "songs", filterLabel: "Duplicate match" });
    setPlayerOpen(true);
    setStatus("Opened existing track");
  }

  async function handleDuplicateReprocess() {
    const duplicate = pendingDuplicateImport?.info;
    if (!duplicate?.trackId) return;
    setPendingDuplicateImport(null);
    await withBusy("Reprocessing duplicate track", async () => {
      await reprocessTrackWithContext(duplicate.trackId!, "Reprocess metadata using the existing source details.");
    });
    await refresh();
  }

  async function handleDuplicateImportAnyway() {
    const duplicateUrl = pendingDuplicateImport?.url;
    if (!duplicateUrl) return;
    setPendingDuplicateImport(null);
    setUrl(duplicateUrl);
    await withBusy("Queueing import anyway", () => queueURLImport(duplicateUrl));
    setUrl("");
  }

  async function handleSaveSettings(scope = "settings") {
    await withBusy("Saving settings", async () => {
      const saved = await saveSettings({
        ...settings,
        apiKey: apiKeyDraft.trim(),
        updateManifestUrl: settings.updateManifestUrl ?? "",
      });
      const normalized = normalizeAppState(saved);
      syncSettingsFromAppState(normalized);
      if (apiKeyDraft.trim()) setApiKeyDraft("");
      setSettingsSaveStatus("Saved");
      setSettingsSaveScope(scope);
      if (settingsSaveTimerRef.current !== null) {
        window.clearTimeout(settingsSaveTimerRef.current);
      }
      settingsSaveTimerRef.current = window.setTimeout(() => {
        setSettingsSaveStatus("");
        setSettingsSaveScope("");
        settingsSaveTimerRef.current = null;
      }, 2400);
    });
  }

  async function handleClearKey() {
    await withBusy("Clearing API key", clearAPIKey);
    setApiKeyDraft("");
  }

  async function handleResetSettings() {
    await withBusy("Resetting settings", async () => {
      const reset = normalizeAppState(await resetSettings());
      syncSettingsFromAppState(reset);
      setApiKeyDraft("");
    });
  }

  async function handleTestYTDLP() {
    const result = await withBusy("Testing yt-dlp", testYTDLP);
    if (result) setStatus(result);
  }

  async function handleTestFFmpeg() {
    const result = await withBusy("Testing ffmpeg", testFFmpeg);
    if (result) setStatus(result);
  }

  async function handleTestAI() {
    const result = await withBusy("Testing AI", testAI);
    if (result) setStatus(result);
  }

  async function handleRescan() {
    await withBusy("Rescanning library", rescanLibrary);
  }

  async function handleRepairLibraryFiles() {
    await withBusy("Repairing library files", repairLibraryFiles);
  }

  async function handleReprocessUnprocessed() {
    await withBusy("Reprocessing unprocessed tracks", reprocessUnprocessedTracks);
  }

  async function handleCheckForUpdates() {
    await withBusy("Checking for updates", checkForUpdates);
  }

  async function handleExportDiagnostics() {
    await withBusy("Exporting diagnostics", exportDiagnostics);
  }

  async function handleOpenUpdateDownload() {
    const url = appState.updateInfo.downloadUrl?.trim();
    if (!url) return;
    await withBusy("Opening update download", () => openURL(url));
  }

  async function handleSelectTrack(track: TrackRecord) {
    setSelectedTrackPath(track.metadataPath);
  }

  async function handleOpenPath(path: string) {
    if (!path) return;
    await withBusy("Opening folder", () => revealPath(path));
  }

  async function handleStartJob(job: Job) {
    await withBusy(job.parentJobId && job.status === "failed" ? "Retrying job" : "Starting job", () =>
      startJob(job.id),
    );
  }

  async function handleStopJob(job: Job) {
    await withBusy("Stopping job", () => stopJob(job.id));
  }

  async function handleDeleteJob(job: Job) {
    await withBusy("Deleting job", () => deleteJob(job.id));
  }

  async function handleRetryFailedPlaylist(job: Job) {
    await withBusy("Retrying failed playlist items", () => retryFailedPlaylistItems(job.id));
  }

  async function handleAddContextAndReprocess(track: TrackRecord, context: string) {
    const trimmed = context.trim();
    if (!trimmed) return;
    setBusy(true);
    setStatus("Reprocessing track with added context");
    try {
      await reprocessTrackWithContext(track.id, trimmed);
      await refresh();
      setStatus("Ready");
      setAddContextOpen(false);
      setAddContextDraft("");
    } catch (error) {
      setStatus(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  }

  async function handleApplyArtwork(track: TrackRecord) {
    const picked = await withBusy("Choosing album art", pickArtworkFile);
    if (!picked) return;
    await withBusy("Applying album art", () => applyArtworkFromFile(track.id, picked));
  }

  async function handlePauseAllJobs() {
    await withBusy("Pausing all jobs", pauseAllJobs);
  }

  async function handleDeleteAllJobs() {
    const confirmed = window.confirm("Delete all jobs? This cannot be undone.");
    if (!confirmed) return;
    await withBusy("Deleting all jobs", deleteAllJobs);
  }

  async function handleDeleteDoneJobs() {
    const confirmed = window.confirm("Delete all completed, failed, and stopped jobs? This cannot be undone.");
    if (!confirmed) return;
    await withBusy("Deleting done jobs", deleteDoneJobs);
  }

  async function handleDeleteQueuedJobs() {
    const confirmed = window.confirm("Delete all queued jobs that have not started yet? This cannot be undone.");
    if (!confirmed) return;
    await withBusy("Deleting queued jobs", deleteQueuedJobs);
  }

  const filteredTracks = useMemo(
    () =>
      filterTracks(tracks, search, {
        lyrics: filterLyrics,
        needsReview: filterNeedsReview,
        missingMetadata: filterMissingMetadata,
        genre: selectedGenre,
        year: selectedYear,
      }),
    [filterLyrics, filterMissingMetadata, filterNeedsReview, search, selectedGenre, selectedYear, tracks],
  );
  const hasTrackFilters = Boolean(
    search.trim() || filterLyrics || filterNeedsReview || filterMissingMetadata || selectedGenre || selectedYear,
  );
  async function playTrack(track: TrackRecord, queueTracks: TrackRecord[] = tracks, source: QueueSource = "library") {
    if (videoOpen || videoFullscreenOpen) {
      await closeVideo(false);
    }
    setSelectedTrackPath(track.metadataPath);
    const next = await backendPlayTrack(
      track.id,
      queueTracks.map((item) => item.id),
      source,
    );
    setPlayer(next);
  }

  async function toggleTrackPlayback(
    track: TrackRecord,
    queueTracks: TrackRecord[] = tracks,
    source: QueueSource = "library",
  ) {
    if (track.id === player.currentTrackId) {
      await togglePlayback();
      return;
    }
    await playTrack(track, queueTracks, source);
  }

  async function openTrackInPlayer(
    track: TrackRecord,
    queueTracks: TrackRecord[] = tracks,
    source: QueueSource = "library",
  ) {
    if (track.id === player.currentTrackId) {
      if (!player.isPlaying) {
        await togglePlayback();
      }
    } else {
      await playTrack(track, queueTracks, source);
    }
    setLyricsOpen(false);
    setQueueOpen(false);
    setPlayerOpen(true);
  }

  async function openTrackVideo(
    track: TrackRecord,
    _queueTracks: TrackRecord[] = tracks,
    _source: QueueSource = "library",
  ) {
    setSelectedTrackPath(track.metadataPath);
    setLyricsOpen(false);
    setQueueOpen(false);
    setPlayerOpen(true);
    await openVideoForTrack(track, true);
  }

  function currentQueueTracks() {
    const queue = Array.isArray(player.queue) ? player.queue : [];
    return queue.map((trackID) => trackById(trackID)).filter(Boolean) as TrackRecord[];
  }

  async function removeTrackFromQueue(trackID: string) {
    const next = await removeTrackFromPlaybackQueue(trackID);
    setPlayer(next);
  }

  async function shuffleCurrentQueue() {
    const next = await shufflePlaybackQueue();
    setPlayer(next);
  }

  async function moveCurrentQueueTrack(trackID: string, delta: number) {
    const next = await moveTrackInPlaybackQueue(trackID, delta);
    setPlayer(next);
  }

  async function togglePlayback() {
    if (videoOpen || videoFullscreenOpen) {
      await handleVideoPlayPause();
      return;
    }
    if (!player.currentTrackId) {
      const nextTrack = selectedTrack ?? tracks[0] ?? null;
      if (nextTrack) {
        await playTrack(nextTrack, tracks, "library");
      }
      return;
    }
    const next = await backendTogglePlayback();
    setPlayer(next);
  }

  async function playNext() {
    if (videoOpen || videoFullscreenOpen) {
      await closeVideo(false);
    }
    const next = await backendPlayNext();
    setPlayer(next);
  }

  async function playPrevious() {
    if (videoOpen || videoFullscreenOpen) {
      await closeVideo(false);
    }
    const next = await backendPlayPrevious();
    setPlayer(next);
  }

  function clearSeekDebounce() {
    if (seekDebounceRef.current !== null) {
      window.clearTimeout(seekDebounceRef.current);
      seekDebounceRef.current = null;
    }
  }

  function commitSeek(ratio: number) {
    if (videoOpen || videoFullscreenOpen) {
      void handleVideoSeek(Math.max(0, ratio) * Math.max(videoDuration || player.duration, 0));
      return;
    }
    const nextRatio = Math.max(0, Math.min(1, ratio));
    setSeekPreviewRatio(nextRatio);
    clearSeekDebounce();
    const requestSeq = ++seekRequestSeqRef.current;
    void backendSeekPlayback(nextRatio)
      .then((next) => {
        if (seekRequestSeqRef.current === requestSeq) {
          setPlayer(next);
          setSeekPreviewRatio(null);
        }
      })
      .catch((error) => {
        if (seekRequestSeqRef.current === requestSeq) {
          setStatus(error instanceof Error ? error.message : String(error));
        }
      });
  }

  function seekTo(ratio: number) {
    if (videoOpen || videoFullscreenOpen) {
      void handleVideoSeek(Math.max(0, Math.min(1, ratio)) * Math.max(videoDuration || player.duration, 0));
      return;
    }
    const nextRatio = Math.max(0, Math.min(1, ratio));
    setSeekPreviewRatio(nextRatio);
    clearSeekDebounce();
    seekDebounceRef.current = window.setTimeout(() => {
      seekDebounceRef.current = null;
      commitSeek(nextRatio);
    }, 90);
  }

  function startSeek(ratio: number) {
    if (videoOpen || videoFullscreenOpen) {
      setVideoCurrentTime(Math.max(0, Math.min(1, ratio)) * Math.max(videoDuration || player.duration, 0));
      return;
    }
    setSeekPreviewRatio(Math.max(0, Math.min(1, ratio)));
  }

  function seekBySeconds(deltaSeconds: number) {
    if (videoOpen || videoFullscreenOpen) {
      const duration = videoDuration > 0 ? videoDuration : player.duration;
      if (duration <= 0) return;
      void handleVideoSeek(Math.max(0, Math.min(duration, videoCurrentTime + deltaSeconds)));
      return;
    }
    const duration = player.duration > 0 ? player.duration : 0;
    if (duration <= 0) return;
    const nextRatio = Math.max(0, Math.min(1, (liveAudioTimeRef.current + deltaSeconds) / duration));
    commitSeek(nextRatio);
  }

  function endSeek(ratio: number) {
    if (videoOpen || videoFullscreenOpen) {
      void handleVideoSeek(Math.max(0, Math.min(1, ratio)) * Math.max(videoDuration || player.duration, 0));
      return;
    }
    commitSeek(ratio);
  }

  function toggleMute() {
    if (videoOpen || videoFullscreenOpen) {
      void handleVideoToggleMute();
      return;
    }
    void backendToggleMute().then((next) => setPlayer(next));
  }

  function setVolume(value: number) {
    if (videoOpen || videoFullscreenOpen) {
      void handleVideoVolume(value);
      return;
    }
    void backendSetPlaybackVolume(Math.max(0, Math.min(1, value))).then((next) => setPlayer(next));
  }

  function toggleShuffle() {
    void backendToggleShuffle().then((state) => setPlayer(state));
  }

  async function saveQueueAsPlaylist() {
    const tracksForQueue = currentQueueTracks();
    if (!tracksForQueue.length) return;
    const playlist = await createPlaylist(
      `Queue ${new Date().toLocaleString()}`,
      "Saved from the current playback queue",
    );
    for (const track of tracksForQueue) {
      await addTrackToPlaylist(playlist.id, track.id);
    }
    await refresh();
    setView("playlists");
    setSelectedPlaylistId(playlist.id);
  }

  function resetBrowseContext(nextView: ViewKey) {
    setBrowseContext({ mode: "library" });
    setBrowseHistory([]);
    setView(nextView);
  }

  function navigateBrowse(nextContext: BrowseContext, nextView: ViewKey) {
    setBrowseHistory((current) => [...current, { context: browseContext, view }]);
    setBrowseContext(nextContext);
    setView(nextView);
  }

  function goBackBrowse() {
    setBrowseHistory((current) => {
      const next = current[current.length - 1];
      if (!next) return current;
      setBrowseContext(next.context);
      setView(next.view);
      return current.slice(0, -1);
    });
  }

  const musicSection = view === "library" || view === "albums" || view === "artists" || view === "songs";
  const currentQueue = currentQueueTracks();
  const playlistMissingCount = useMemo(
    () => appState.playlists.reduce((sum, playlist) => sum + playlist.trackIds.filter((trackID) => !tracksByID.has(trackID)).length, 0),
    [appState.playlists, tracksByID],
  );
  const queueMissingCount = useMemo(
    () => (Array.isArray(player.queue) ? player.queue.filter((trackID) => !tracksByID.has(trackID)).length : 0),
    [player.queue, tracksByID],
  );
  const focusTrack = currentTrack ?? selectedTrack ?? null;
  const lyricsPanelTrack = currentTrack ?? selectedTrack ?? null;
  const videoTrack = focusTrack;
  const videoSourcePath = videoTrack?.videoPath?.trim() ?? "";
  const videoSource = videoSourceUrl.trim();
  const videoSourceType =
    videoSourceTypeFromServer || (videoSourcePath ? mimeTypeFromPath(videoSourcePath) : "video/mp4");
  const videoDiagnosticSource = videoDiagnosticSourceType(
    videoSourcePath,
    videoSource,
    videoTrack?.sourceRef ?? "",
  );
  const videoDiagnosticJobID =
    appState.jobs.find((job) => job.trackId === videoTrack?.id && job.downloadVideo)?.id ?? "";
  const videoDiagnosticMode = videoFullscreenOpen ? "fullscreen" : videoOpen ? "inline" : "closed";

  useEffect(() => {
    setPlayerInfoOpen(false);
    setAddContextOpen(false);
    setAddContextDraft("");
  }, [focusTrack?.id]);

  useEffect(() => {
    if (!playerOpen) {
      setPlayerInfoOpen(false);
      setAddContextOpen(false);
      setAddContextDraft("");
    }
  }, [playerOpen]);

  return (
    <main
      className={`app-shell ${lyricsOpen || queueOpen ? "panel-open" : ""} ${playerOpen ? "player-open" : ""}`}
      onMouseMoveCapture={handleTooltipMove}
      onMouseLeave={handleTooltipLeave}
    >
      <ModernSidebar
        state={appState}
        section={view}
        setSection={(nextView) => {
          if (nextView === "library" || nextView === "artists" || nextView === "albums" || nextView === "songs") {
            resetBrowseContext(nextView);
            return;
          }
          setView(nextView);
        }}
        search={search}
        setSearch={setSearch}
        busy={busy}
        onOpenSettings={() => setView("settings")}
        onToggleWindowMaximize={handleToggleWindowMaximize}
      />

      <section className="workspace">
        {playerOpen ? (
          <PlaybackTime
            player={player}
            videoOpen={videoOpen}
            videoFullscreenOpen={videoFullscreenOpen}
            videoCurrentTime={videoCurrentTime}
            videoDuration={videoDuration}
            seekPreviewRatio={seekPreviewRatio}
            liveAudioTimeRef={liveAudioTimeRef}
          >
            {({ seekDisplayCurrentTime, seekDisplayRatio }) => (
              <ImmersivePlayerView
                track={focusTrack}
                preview={preview}
                previewLoading={previewLoading}
                artworkDataUrl={
                  preview?.track.id === focusTrack?.id
                    ? (preview?.artworkDataUrl ?? focusTrack?.artworkDataUrl ?? "")
                    : (focusTrack?.artworkDataUrl ?? "")
                }
                videoSource={videoSource}
                videoSourceType={videoSourceType}
                videoShouldPlay={videoShouldPlay}
                videoOpen={videoOpen}
                videoFullscreenOpen={videoFullscreenOpen}
                videoLoading={videoLoading}
                videoError={videoError}
                videoCurrentTime={videoCurrentTime}
                videoDuration={videoDuration}
                videoIsPlaying={videoIsPlaying}
                videoVolume={videoVolume}
                videoMuted={videoMuted}
                hasVideoDownloaded={videoTrackAvailable}
                videoPlayerRef={videoPlayerRef}
                currentTime={seekDisplayCurrentTime}
                duration={player.duration}
                seekValue={seekDisplayRatio}
                isPlaying={player.isPlaying}
                isLoading={player.isLoading}
                volume={player.volume}
                muted={player.muted}
                shuffleEnabled={player.shuffleEnabled}
                repeatMode={normalizeRepeatMode(player.repeatMode)}
                error={player.error}
                onClose={() => setPlayerOpen(false)}
                onBack={() => setPlayerOpen(false)}
                onPrevious={playPrevious}
                onPlayPause={togglePlayback}
                onNext={playNext}
                onSeek={seekTo}
                onSeekStart={startSeek}
                onSeekEnd={endSeek}
                onVolume={setVolume}
                onToggleMute={toggleMute}
                onToggleShuffle={() => toggleShuffle()}
                onToggleRepeat={() => void backendToggleRepeatMode().then((next) => setPlayer(next))}
                onOpenInfo={() => setPlayerInfoOpen(true)}
                onOpenVideo={() => (focusTrack ? openVideoForTrack(focusTrack, true) : Promise.resolve())}
                onCloseVideo={() => closeVideo(true)}
                onDownloadVideo={() => (focusTrack ? openVideoForTrack(focusTrack, true) : Promise.resolve())}
                onSetArtwork={() => (focusTrack ? handleApplyArtwork(focusTrack) : Promise.resolve())}
                onVideoTimeChange={(value) => setVideoCurrentTime(value)}
                onVideoDurationChange={(value) => setVideoDuration(value)}
                onVideoPlayingChange={(value) => setVideoIsPlaying(value)}
                onVideoError={(event) => {
                  setVideoLoading(false);
                  setVideoShouldPlay(false);
                  setVideoIsPlaying(false);
                  const value = `Video playback failed${event.playerErrorCode ? ` (code ${event.playerErrorCode})` : ""}${event.playerErrorMessage ? `: ${event.playerErrorMessage}` : ""}`;
                  setVideoError(value);
                  void logVideoEvent(
                    "videojs_error",
                    `track:${videoTrack?.id || "unknown"}`,
                    serializeVideoDiagnosticContext({
                      trackId: videoTrack?.id,
                      jobId: videoDiagnosticJobID,
                      sourceType: videoDiagnosticSource,
                      mimeType: event.mimeType || videoSourceType,
                      playerErrorCode: event.playerErrorCode,
                      browserEvent: event.browserEvent,
                      mode: videoDiagnosticMode,
                      downloadMode: appState.settings.videoDownloadMode,
                      message: value,
                      sourcePathAvailable: Boolean(videoSourcePath),
                      remoteSourceAvailable: Boolean(videoTrack?.sourceRef?.trim()),
                    }),
                  );
                  if (String(event.playerErrorCode) === "4" && focusTrack && focusTrack.sourceRef.trim() && !videoLoading) {
                    void openVideoForTrack(focusTrack, true, true);
                  }
                }}
                onVideoLoaded={() => setVideoLoading(false)}
                onAddToPlaylist={() => {
                  if (focusTrack) {
                    setPlaylistTrackTarget(focusTrack);
                    setPlaylistModalOpen(true);
                  }
                }}
                onAddContext={() => {
                  setAddContextDraft("");
                  setAddContextOpen(true);
                }}
                onRevealFolder={() => {
                  if (focusTrack) {
                    void handleOpenPath(focusTrack.storageDir);
                  }
                }}
              />
            )}
          </PlaybackTime>
        ) : (
          <>
            {!musicSection ? (
              <header className="page-bar">
                <div>
                  <h1>{pageTitle(view)}</h1>
                  {pageSubtitle(view) ? <p className="page-subtitle">{pageSubtitle(view)}</p> : null}
                </div>
                <div className="top-status">
                  <div className="top-status-line">{status}</div>
                  <div className="top-status-meta">
                    {appState.aiStatus ||
                      `${appState.stats.trackCount} tracks · ${appState.stats.albumCount} albums · ${appState.health.readyTracks} ready`}
                  </div>
                </div>
              </header>
            ) : null}

            {view === "import" ? (
              <ImportPageModern
                url={url}
                setUrl={setUrl}
                busy={busy}
                onQueueURL={handleQueueURL}
                onChooseFiles={handleChooseFiles}
                onChooseFolder={handleChooseFolder}
                onChooseRoot={handleChooseRoot}
                onRescan={handleRescan}
                onOpenSettings={() => setView("settings")}
                onToggleWindowMaximize={handleToggleWindowMaximize}
                state={appState}
                jobs={appState.jobs}
                importHistory={appState.importHistory}
                selectedJobId={selectedJobId}
                onSelectJob={setSelectedJobId}
                onStartJob={handleStartJob}
                onStopJob={handleStopJob}
                onDeleteJob={handleDeleteJob}
                onRetryFailedPlaylist={handleRetryFailedPlaylist}
                onPauseAllJobs={handlePauseAllJobs}
                onDeleteAllJobs={handleDeleteAllJobs}
                onDeleteDoneJobs={handleDeleteDoneJobs}
                onDeleteQueuedJobs={handleDeleteQueuedJobs}
                onClearImportHistory={handleClearImportHistory}
                advancedOpen={advancedOpen}
                setAdvancedOpen={setAdvancedOpen}
              />
            ) : null}

            {view === "index" ? (
              <LibraryIndexPage
                state={appState}
                tracks={tracks}
                search={search}
                filteredTracks={filteredTracks}
                hasTrackFilters={hasTrackFilters}
                genreBuckets={appState.genreBuckets}
                yearBuckets={appState.yearBuckets}
                selectedGenre={selectedGenre}
                setSelectedGenre={setSelectedGenre}
                selectedYear={selectedYear}
                setSelectedYear={setSelectedYear}
                filterLyrics={filterLyrics}
                setFilterLyrics={setFilterLyrics}
                filterNeedsReview={filterNeedsReview}
                setFilterNeedsReview={setFilterNeedsReview}
                filterMissingMetadata={filterMissingMetadata}
                setFilterMissingMetadata={setFilterMissingMetadata}
                selectedTrackPath={selectedTrackPath}
                currentTrackId={player.currentTrackId}
                currentTrackPlaying={player.isPlaying}
                queueTracks={currentQueue}
                playlists={appState.playlists}
                playlistMissingCount={playlistMissingCount}
                queueMissingCount={queueMissingCount}
                onSelectTrack={handleSelectTrack}
                onToggleTrackPlayback={toggleTrackPlayback}
                onOpenTrackPlayer={openTrackInPlayer}
                onOpenTrackVideo={openTrackVideo}
                onOpenLyrics={(track) => {
                  handleSelectTrack(track);
                  setLyricsOpen(true);
                  setQueueOpen(false);
                }}
                onAddToPlaylist={(track) => {
                  setPlaylistTrackTarget(track);
                  setPlaylistModalOpen(true);
                }}
                onRevealTrack={handleOpenPath}
                onRescan={handleRescan}
                onRepairLibraryFiles={handleRepairLibraryFiles}
                onReprocessUnprocessed={handleReprocessUnprocessed}
              />
            ) : null}

            {musicSection ? (
              <MusicBrowserPage
                section={view}
                browseContext={browseContext}
                browseHistory={browseHistory}
                tracks={tracks}
                filteredTracks={filteredTracks}
                hasTrackFilters={hasTrackFilters}
                search={search}
                setSearch={setSearch}
                state={appState}
                genreBuckets={appState.genreBuckets}
                yearBuckets={appState.yearBuckets}
                filterLyrics={filterLyrics}
                setFilterLyrics={setFilterLyrics}
                filterNeedsReview={filterNeedsReview}
                setFilterNeedsReview={setFilterNeedsReview}
                filterMissingMetadata={filterMissingMetadata}
                setFilterMissingMetadata={setFilterMissingMetadata}
                selectedGenre={selectedGenre}
                setSelectedGenre={setSelectedGenre}
                selectedYear={selectedYear}
                setSelectedYear={setSelectedYear}
                selectedTrackPath={selectedTrackPath}
                currentTrackId={player.currentTrackId}
                currentTrackPlaying={player.isPlaying}
                onSelectTrack={handleSelectTrack}
                onPlayTrack={playTrack}
                onToggleTrackPlayback={toggleTrackPlayback}
                onOpenTrackPlayer={openTrackInPlayer}
                onOpenTrackVideo={openTrackVideo}
                onOpenLyrics={(track) => {
                  handleSelectTrack(track);
                  setLyricsOpen(true);
                  setQueueOpen(false);
                }}
                onAddToPlaylist={(track) => {
                  setPlaylistTrackTarget(track);
                  setPlaylistModalOpen(true);
                }}
                onRevealTrack={handleOpenPath}
                onOpenPlayer={() => {
                  setLyricsOpen(false);
                  setQueueOpen(false);
                  setPlayerOpen(true);
                }}
                onOpenImport={() => setView("import")}
                onOpenSettings={() => setView("settings")}
                onNavigateBrowse={navigateBrowse}
                onBackBrowse={goBackBrowse}
                onResetBrowse={resetBrowseContext}
              />
            ) : null}

            {view === "processing" ? (
              <ProcessingPageModern
                jobs={appState.jobs}
                selectedJobId={selectedJobId}
                onSelectJob={setSelectedJobId}
                onStartJob={handleStartJob}
                onStopJob={handleStopJob}
                onDeleteJob={handleDeleteJob}
                onRetryFailedPlaylist={handleRetryFailedPlaylist}
                onPauseAllJobs={handlePauseAllJobs}
                onDeleteAllJobs={handleDeleteAllJobs}
                onDeleteDoneJobs={handleDeleteDoneJobs}
                onDeleteQueuedJobs={handleDeleteQueuedJobs}
                state={appState}
              />
            ) : null}

            {view === "playlists" ? (
              <PlaylistsPageModern
                playlists={appState.playlists}
                tracks={tracks}
                selectedPlaylistId={selectedPlaylistId}
                setSelectedPlaylistId={setSelectedPlaylistId}
                onOpenCreatePlaylist={() => {
                  setPlaylistCreateOpen(true);
                  setPlaylistDraftName("");
                  setPlaylistDraftDescription("");
                }}
                onRequestRenamePlaylist={(playlist) => {
                  setPlaylistEditOpen(true);
                  setPlaylistEditId(playlist.id);
                  setPlaylistEditName(playlist.name);
                  setPlaylistEditDescription(playlist.description);
                }}
                onDeletePlaylist={async (playlistID) => {
                  await deletePlaylist(playlistID);
                  await refresh();
                }}
                onPlayTrack={playTrack}
                onAddToPlaylist={async (playlistID, trackID) => {
                  await addTrackToPlaylist(playlistID, trackID);
                  await refresh();
                }}
                onMoveTrack={async (playlistID, trackID, delta) => {
                  await movePlaylistTrack(playlistID, trackID, delta);
                  await refresh();
                }}
                onRemoveTrack={async (playlistID, trackID) => {
                  await removeTrackFromPlaylist(playlistID, trackID);
                  await refresh();
                }}
                onOpenTrackDetail={(track) => {
                  setView("library");
                  setSelectedTrackPath(track.metadataPath);
                  setLyricsOpen(true);
                }}
              />
            ) : null}

            {view === "settings" ? (
              <SettingsPageModern
                settings={settings}
                setSettings={setSettings}
                apiKeyDraft={apiKeyDraft}
                setApiKeyDraft={setApiKeyDraft}
                state={appState}
                saveStatusMessage={settingsSaveStatus}
                saveStatusScope={settingsSaveScope}
                statusMessage={status}
                busy={busy}
                onChooseRoot={handleChooseRoot}
                onSaveSettings={handleSaveSettings}
                onClearKey={handleClearKey}
                onResetSettings={handleResetSettings}
                onTestYTDLP={handleTestYTDLP}
                onTestFFmpeg={handleTestFFmpeg}
                onTestAI={handleTestAI}
                onCheckForUpdates={handleCheckForUpdates}
                onExportDiagnostics={handleExportDiagnostics}
                onOpenUpdateDownload={handleOpenUpdateDownload}
                onOpenDiagnosticsFolder={() => handleOpenPath(appState.diagnostics.lastExportPath)}
              />
            ) : null}
          </>
        )}

        {!playerOpen && view !== "settings" ? (
          <PlaybackTime
            player={player}
            videoOpen={videoOpen}
            videoFullscreenOpen={videoFullscreenOpen}
            videoCurrentTime={videoCurrentTime}
            videoDuration={videoDuration}
            seekPreviewRatio={seekPreviewRatio}
            liveAudioTimeRef={liveAudioTimeRef}
          >
            {({ audioCurrentTime, seekDisplayRatio }) => (
              <CompactPlayerBar
                stacked={lyricsOpen || queueOpen}
                track={currentTrack}
                artworkDataUrl={
                  preview?.track.id === currentTrack?.id
                    ? (preview?.artworkDataUrl ?? currentTrack?.artworkDataUrl ?? "")
                    : (currentTrack?.artworkDataUrl ?? "")
                }
                currentTime={audioCurrentTime}
                duration={player.duration}
                seekValue={seekDisplayRatio}
                isPlaying={player.isPlaying}
                isLoading={player.isLoading}
                muted={player.muted}
                volume={player.volume}
                shuffleEnabled={player.shuffleEnabled}
                repeatMode={normalizeRepeatMode(player.repeatMode)}
                error={player.error}
                queueLength={Array.isArray(player.queue) ? player.queue.length : 0}
                lyricsOpen={lyricsOpen}
                hasVideoDownloaded={videoTrackAvailable}
                onOpenPlayer={() => {
                  setLyricsOpen(false);
                  setQueueOpen(false);
                  setPlayerOpen(true);
                }}
                onOpenCurrentTrack={() => {
                  if (currentTrack) {
                    setSelectedTrackPath(currentTrack.metadataPath);
                    setView("library");
                  }
                  setLyricsOpen(false);
                  setQueueOpen(false);
                  setPlayerOpen(true);
                }}
                onPrevious={playPrevious}
                onPlayPause={togglePlayback}
                onNext={playNext}
                onSeek={seekTo}
                onSeekStart={startSeek}
                onSeekEnd={endSeek}
                onVolume={setVolume}
                onToggleMute={toggleMute}
                onToggleLyrics={() => {
                  setQueueOpen(false);
                  setLyricsOpen((current) => !current);
                }}
                onToggleQueue={() => {
                  setLyricsOpen(false);
                  setQueueOpen((current) => !current);
                }}
                onToggleShuffle={() => toggleShuffle()}
                onToggleRepeat={() => void backendToggleRepeatMode().then((next) => setPlayer(next))}
                onOpenPlaylistPicker={() => {
                  if (currentTrack) {
                    setPlaylistTrackTarget(currentTrack);
                    setPlaylistModalOpen(true);
                  }
                }}
              />
            )}
          </PlaybackTime>
        ) : null}
      </section>

      {lyricsOpen ? (
        <PlaybackTime
          player={player}
          videoOpen={videoOpen}
          videoFullscreenOpen={videoFullscreenOpen}
          videoCurrentTime={videoCurrentTime}
          videoDuration={videoDuration}
          seekPreviewRatio={seekPreviewRatio}
          liveAudioTimeRef={liveAudioTimeRef}
        >
          {({ audioCurrentTime }) => (
            <aside className="right-panel">
              <LyricsSidePanel
                track={lyricsPanelTrack}
                preview={preview}
                previewLoading={previewLoading}
                previewStatus={previewStatus}
                currentTime={audioCurrentTime}
                onClose={() => setLyricsOpen(false)}
                onOpenPlayer={() => {
                  setLyricsOpen(false);
                  setPlayerOpen(true);
                }}
                onOpenFolder={handleOpenPath}
                onOpenURL={openURL}
                onOpenTxt={() => {
                  const filePath = preview?.fileStates?.find((file) => file.label === "TXT")?.path;
                  if (filePath) {
                    void handleOpenPath(filePath);
                  }
                }}
                onOpenLrc={() => {
                  const filePath = preview?.fileStates?.find((file) => file.label === "LRC")?.path;
                  if (filePath) {
                    void handleOpenPath(filePath);
                  }
                }}
              />
            </aside>
          )}
        </PlaybackTime>
      ) : queueOpen ? (
        <aside className="right-panel">
          <PlaybackQueueDrawer
            queueTracks={currentQueue}
            currentTrackId={player.currentTrackId}
            onClose={() => setQueueOpen(false)}
            onClearQueue={() => void clearPlaybackQueue().then((next) => setPlayer(next))}
            onSaveQueue={saveQueueAsPlaylist}
            onPlayTrack={playTrack}
            onShuffleQueue={shuffleCurrentQueue}
            onRemoveFromQueue={removeTrackFromQueue}
            onMoveTrack={(trackID, delta) => void moveCurrentQueueTrack(trackID, delta)}
            onAddToPlaylist={async (playlistID, trackID) => {
              await addTrackToPlaylist(playlistID, trackID);
              await refresh();
            }}
            playlists={appState.playlists}
          />
        </aside>
      ) : null}

      {playlistCreateOpen ? (
        <PlaylistCreateModal
          title="New playlist"
          ctaLabel="Create playlist"
          draftName={playlistCreateName}
          setDraftName={setPlaylistCreateName}
          draftDescription={playlistCreateDescription}
          setDraftDescription={setPlaylistCreateDescription}
          onClose={() => {
            setPlaylistCreateOpen(false);
            setPlaylistCreateName("");
            setPlaylistCreateDescription("");
          }}
          onCreate={async () => {
            const name = playlistCreateName.trim();
            if (!name) return;
            const playlist = await createPlaylist(name, playlistCreateDescription);
            await refresh();
            setSelectedPlaylistId(playlist.id);
            setView("playlists");
            setPlaylistCreateOpen(false);
            setPlaylistCreateName("");
            setPlaylistCreateDescription("");
          }}
        />
      ) : null}

      {playlistEditOpen ? (
        <PlaylistCreateModal
          title="Edit playlist"
          ctaLabel="Save changes"
          draftName={playlistEditName}
          setDraftName={setPlaylistEditName}
          draftDescription={playlistEditDescription}
          setDraftDescription={setPlaylistEditDescription}
          onClose={() => {
            setPlaylistEditOpen(false);
            setPlaylistEditId("");
            setPlaylistEditName("");
            setPlaylistEditDescription("");
          }}
          onCreate={async () => {
            if (!playlistEditId) return;
            await renamePlaylist(playlistEditId, playlistEditName, playlistEditDescription);
            await refresh();
            setPlaylistEditOpen(false);
            setPlaylistEditId("");
            setPlaylistEditName("");
            setPlaylistEditDescription("");
          }}
        />
      ) : null}

      {playlistModalOpen ? (
        <AddToPlaylistModal
          playlists={appState.playlists}
          track={playlistTrackTarget}
          onClose={() => {
            setPlaylistModalOpen(false);
            setPlaylistDraftName("");
            setPlaylistDraftDescription("");
            setPlaylistTrackTarget(null);
          }}
          onCreate={async (name, description) => {
            const playlist = await createPlaylist(name, description);
            if (playlistTrackTarget) {
              await addTrackToPlaylist(playlist.id, playlistTrackTarget.id);
            }
            await refresh();
            setSelectedPlaylistId(playlist.id);
            setView("playlists");
            setPlaylistModalOpen(false);
          }}
          onAddExisting={async (playlistID) => {
            if (playlistTrackTarget) {
              await addTrackToPlaylist(playlistID, playlistTrackTarget.id);
            }
            await refresh();
            setPlaylistModalOpen(false);
          }}
          draftName={playlistDraftName}
          setDraftName={setPlaylistDraftName}
          draftDescription={playlistDraftDescription}
          setDraftDescription={setPlaylistDraftDescription}
        />
      ) : null}

      {playerOpen && playerInfoOpen && focusTrack ? (
        <TrackProfileModal
          track={focusTrack}
          preview={preview}
          onClose={() => setPlayerInfoOpen(false)}
          onOpenURL={openURL}
        />
      ) : null}

      {playerOpen && addContextOpen && focusTrack ? (
        <TrackContextModal
          track={focusTrack}
          userContext={addContextDraft}
          setUserContext={setAddContextDraft}
          aiReady={appState.settings.apiKeyConfigured}
          onClose={() => {
            setAddContextOpen(false);
            setAddContextDraft("");
          }}
          onSubmit={async () => {
            await handleAddContextAndReprocess(focusTrack, addContextDraft);
          }}
        />
      ) : null}

      {pendingDuplicateImport ? (
        <DuplicateImportModal
          info={pendingDuplicateImport.info}
          sourceUrl={pendingDuplicateImport.url}
          onClose={() => setPendingDuplicateImport(null)}
          onOpenExisting={handleDuplicateOpenExisting}
          onReprocess={handleDuplicateReprocess}
          onImportAnyway={handleDuplicateImportAnyway}
        />
      ) : null}

      {hoverTooltip ? (
        <div
          ref={tooltipRef}
          className={`hover-tooltip ${hoverTooltip.visible ? "visible" : ""} ${hoverTooltip.flip ? "flip" : ""}`}
          style={{ left: hoverTooltip.x, top: hoverTooltip.y }}
          aria-hidden="true"
        >
          {hoverTooltip.text}
        </div>
      ) : null}
    </main>
  );
}

function TrackContextModal({
  track,
  userContext,
  setUserContext,
  aiReady,
  onClose,
  onSubmit,
}: {
  track: TrackRecord;
  userContext: string;
  setUserContext: (value: string) => void;
  aiReady: boolean;
  onClose: () => void;
  onSubmit: () => Promise<void>;
}) {
  const canSubmit = aiReady && userContext.trim().length > 0;

  return (
    <div className="modal-backdrop modal-backdrop-soft" onClick={onClose}>
      <div className="modal modal-track-context" onClick={(event) => event.stopPropagation()}>
        <div className="modal-head">
          <div>
            <div className="modal-kicker">Add context</div>
            <div className="modal-title">{track.title}</div>
            <div className="modal-subtitle">
              {track.artist} · {track.album}
            </div>
          </div>
          <button className="icon-button" onClick={onClose} aria-label="Close add context">
            ✕
          </button>
        </div>
        <div className="detail-note">
          Add any extra context that helps Melodex identify the track, album, release, or version more accurately. The
          audio file will not be redownloaded.
        </div>
        <label className="input-group">
          <span>Additional context</span>
          <textarea
            className="text-input modal-textarea modal-context-textarea"
            value={userContext}
            onChange={(event) => setUserContext(event.target.value)}
            placeholder="This is artist name, from album name, live version, radio edit, or any other clues that help the reprocess pass."
          />
        </label>
        {!aiReady ? (
          <div className="detail-note">An AI key is required for this feature to re-run metadata discovery.</div>
        ) : null}
        <div className="action-row modal-actions">
          <button className="button button-secondary" onClick={onClose}>
            Cancel
          </button>
          <button className="button button-primary" onClick={() => void onSubmit()} disabled={!canSubmit}>
            Add Context and Reprocess
          </button>
        </div>
      </div>
    </div>
  );
}

function DuplicateImportModal({
  info,
  sourceUrl,
  onClose,
  onOpenExisting,
  onReprocess,
  onImportAnyway,
}: {
  info: URLImportDuplicateInfo;
  sourceUrl: string;
  onClose: () => void;
  onOpenExisting: () => Promise<void>;
  onReprocess: () => Promise<void>;
  onImportAnyway: () => Promise<void>;
}) {
  const title = info.title || "Existing track";
  const artist = info.artist || "Unknown artist";
  const album = info.album || "Unknown album";

  return (
    <div className="modal-backdrop modal-backdrop-soft" onClick={onClose}>
      <div className="modal modal-track-context" onClick={(event) => event.stopPropagation()}>
        <div className="modal-head">
          <div>
            <div className="modal-kicker">Duplicate import detected</div>
            <div className="modal-title">{title}</div>
            <div className="modal-subtitle">
              {artist} · {album}
            </div>
          </div>
          <button className="icon-button" onClick={onClose} aria-label="Close duplicate dialog">
            ✕
          </button>
        </div>
        <div className="detail-note">
          {info.matchReason ? `${info.matchReason}. ` : ""}
          This URL appears to already be in your library or queue.
        </div>
        <div className="detail-group">
          <KeyValue label="Requested URL" value={sourceUrl} />
          <KeyValue label="Existing URL" value={info.sourceUrl || info.videoUrl || "Unknown"} />
          <KeyValue label="Existing track" value={info.metadataPath || info.jobId || info.trackId || "Unknown"} />
        </div>
        <div className="action-row modal-actions">
          <button className="button button-secondary" onClick={() => void onOpenExisting()}>
            Open existing
          </button>
          <button className="button button-secondary" onClick={() => void onReprocess()}>
            Reprocess metadata
          </button>
          <button className="button button-primary" onClick={() => void onImportAnyway()}>
            Import anyway
          </button>
        </div>
      </div>
    </div>
  );
}
