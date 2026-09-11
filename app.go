package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	sysruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"melodex/internal/songstore"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type App struct {
	wailsApp                 *application.App
	window                   application.Window
	appCtx                   context.Context
	mu                       sync.Mutex
	store                    *Store
	info                     RootInfo
	settings                 storedSettings
	buildInfo                BuildInfo
	updateInfo               UpdateInfo
	diagnostics              DiagnosticsInfo
	catalog                  catalogFile
	playlists                []Playlist
	importHistory            []ImportHistoryEntry
	player                   *playbackEngine
	jobs                     []Job
	jobCancels               map[string]context.CancelFunc
	trackIndex               map[string]TrackRecord
	jobQueue                 []string
	mediaMu                  sync.Mutex
	mediaServer              *mediaServer
	workerMu                 sync.Mutex
	workerStates             map[int]*workerState
	workerNextID             int
	workerAdjust             chan struct{}
	throughputMu             sync.Mutex
	activeVideoDownloads     int
	activeEnrichmentRequests int
	activeArtworkRequests    int
	activeLyricsRequests     int
	stateEmitMu              sync.Mutex
	stateEmitTimer           *time.Timer
	downloadStats            downloadStats
	sessionUA                string
	queueFrozen              bool
	promptLibrary            *PromptLibrary
	ai                       *aiClient
	pipelineCache            *PipelineCache
	aiStatus                 string
	toolReadinessCache       map[string]toolReadinessResult
}

func NewApp() *App {
	prompts, _ := loadPromptLibrary()
	buildInfo := currentBuildInfo()
	root, err := loadRootPreference()
	if err != nil || root == "" {
		root = defaultLibraryRoot()
	}
	store, info, err := newStore(root)
	if err != nil {
		info = RootInfo{LibraryRoot: root}
		store, _, _ = newStore(root)
	}
	settings, _ := store.loadSettings()
	if settings.LibraryRoot == "" {
		settings.LibraryRoot = info.LibraryRoot
	}
	catalog, _ := loadCachedLibraryCatalogSnapshot(store)
	playlists, _ := store.loadPlaylists()
	importHistory, _ := store.loadImportHistory()
	app := &App{
		store:         store,
		info:          info,
		settings:      settings,
		buildInfo:     buildInfo,
		updateInfo:    defaultUpdateInfo(settings, buildInfo),
		diagnostics:   defaultDiagnosticsInfo(),
		catalog:       catalog,
		playlists:     playlists.Playlists,
		importHistory: importHistory.Entries,
		player:        newPlaybackEngine(),
		trackIndex:    map[string]TrackRecord{},
		jobCancels:    map[string]context.CancelFunc{},
		jobQueue:      []string{},
		workerStates:  map[int]*workerState{},
		workerAdjust:  make(chan struct{}, 1),
		sessionUA:     makeSessionUserAgent(),
		promptLibrary: prompts,
		aiStatus:      aiStatusForSettings(settings, prompts),
	}
	if cache, err := loadPipelineCache(store.pipelineCachePath); err == nil {
		app.pipelineCache = cache
	} else {
		log.Printf("startup: pipeline cache load failed: %v", err)
		logEvent("pipeline_cache_load_failed", "error", err.Error())
		app.pipelineCache = &PipelineCache{path: store.pipelineCachePath}
	}
	app.rebuildTrackIndexLocked()
	app.player.setLookup(app.trackIndex)
	app.ai = newAIClient(app.settings)
	app.toolReadinessCache = app.initialToolReadiness()
	_ = saveRootPreference(app.settings.LibraryRoot)
	return app
}

func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.appCtx = ctx
	a.startup(ctx)
	return nil
}

func (a *App) startup(ctx context.Context) {
	log.Printf("startup: initializing UI and background workers")
	log.Printf("startup: Melodex %s", buildSummary())
	log.Printf("startup: session user-agent %s", a.sessionUA)
	logEvent("startup", "build", buildSummary(), "library_root", a.info.LibraryRoot, "app_data_dir", a.info.AppDataDir, "incoming_dir", a.info.IncomingDir)
	if a.promptLibrary == nil {
		prompts, err := loadPromptLibrary()
		if err == nil {
			a.promptLibrary = prompts
		}
	}
	if recovered, interrupted, err := a.recoverStartupJobs(); err != nil {
		log.Printf("startup: job recovery failed: %v", err)
		logEvent("startup_job_recovery_failed", "error", err.Error())
	} else if recovered > 0 {
		log.Printf("startup: recovered %d job(s), interrupted %d running job(s)", recovered, interrupted)
		logEvent("startup_job_recovered", "job_count", recovered, "interrupted_count", interrupted)
	}
	if err := a.cleanupIncomingArtifacts(); err != nil {
		log.Printf("startup: incoming cleanup failed: %v", err)
		logEvent("startup_cleanup_failed", "operation", "incoming_cleanup", "error", err.Error())
	}
	if err := a.ensureMediaServer(); err != nil {
		log.Printf("startup: media server failed: %v", err)
		logEvent("media_server_failed", "operation", "startup", "error", err.Error())
	} else {
		a.mediaMu.Lock()
		baseURL := ""
		if a.mediaServer != nil {
			baseURL = a.mediaServer.baseURL
		}
		a.mediaMu.Unlock()
		logEvent("media_server_started", "base_url", baseURL)
	}
	go a.runWorkerManager(ctx)
	a.emitState()
	go a.reconcileLibraryFromDisk()
}

func (a *App) recoverStartupJobs() (int, int, error) {
	if a == nil {
		return 0, 0, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.catalog.Jobs) == 0 {
		a.jobs = []Job{}
		a.jobQueue = []string{}
		a.queueFrozen = false
		return 0, 0, nil
	}
	now := time.Now().UTC()
	interruptedRoots := map[string]struct{}{}
	for _, job := range a.catalog.Jobs {
		if job.Kind == "playlist" && job.ParentJobID == "" && job.Status == "running" {
			interruptedRoots[job.ID] = struct{}{}
		}
	}
	recovered := make([]Job, 0, len(a.catalog.Jobs))
	recoveredQueue := make([]string, 0, len(a.catalog.Jobs))
	interrupted := 0
	changed := false
	for _, original := range a.catalog.Jobs {
		job := original
		needsInterrupt := false
		if job.Status == "running" {
			needsInterrupt = true
		}
		if _, ok := interruptedRoots[job.ParentJobID]; ok && job.Status == "queued" {
			needsInterrupt = true
		}
		if _, ok := interruptedRoots[job.ParentJobID]; ok && job.Status == "running" {
			needsInterrupt = true
		}
		if job.Kind == "playlist" && job.ParentJobID == "" && job.Status == "running" {
			needsInterrupt = true
		}
		if needsInterrupt {
			interrupted++
			changed = true
			job.Status = "stopped"
			job.Error = "Interrupted by application restart"
			if job.Kind == "playlist" && job.ParentJobID == "" {
				job.Detail = "This is a playlist. Processing time extended. Interrupted by application restart."
			} else {
				job.Detail = "Interrupted by application restart"
			}
			job.FinishedAt = now
		}
		if job.Status == "queued" {
			recoveredQueue = append(recoveredQueue, job.ID)
		}
		job.StageStatuses = jobStageStatusesFor(job)
		recovered = append(recovered, job)
	}
	a.jobs = recovered
	a.catalog.Jobs = append([]Job(nil), recovered...)
	a.jobQueue = append([]string(nil), recoveredQueue...)
	a.queueFrozen = false
	if changed {
		a.persistLocked()
	}
	a.logQueueStateLocked("queue_state_changed", "reason", "startup_job_recovery", "recovered_jobs", len(recovered), "interrupted_jobs", interrupted)
	return len(recovered), interrupted, nil
}

func (a *App) cleanupIncomingArtifacts() error {
	if a == nil || a.info.IncomingDir == "" {
		return nil
	}
	entries, err := os.ReadDir(a.info.IncomingDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cleaned := 0
	preserve := map[string]struct{}{}
	a.mu.Lock()
	for _, job := range a.jobs {
		if job.Status == "completed" {
			continue
		}
		preserve[job.ID] = struct{}{}
	}
	a.mu.Unlock()
	for _, entry := range entries {
		if _, ok := preserve[entry.Name()]; ok {
			continue
		}
		target := filepath.Join(a.info.IncomingDir, entry.Name())
		if err := os.RemoveAll(target); err != nil {
			log.Printf("startup: failed to remove stale incoming artifact %s: %v", target, err)
			continue
		}
		cleaned++
	}
	if cleaned > 0 {
		log.Printf("startup: cleaned %d stale incoming artifact(s)", cleaned)
		logEvent("startup_cleanup_completed", "artifact_count", cleaned, "incoming_dir", a.info.IncomingDir)
	}
	return nil
}

func makeSessionUserAgent() string {
	seed := time.Now().UnixNano() ^ int64(os.Getpid())
	rng := rand.New(rand.NewSource(seed))
	platform := "Macintosh; Intel Mac OS X 10_15_7"
	switch sysruntime.GOOS {
	case "windows":
		platform = "Windows NT 10.0; Win64; x64"
	case "linux":
		platform = "X11; Linux x86_64"
	}
	major := 143 + rng.Intn(8)
	build := 1000 + rng.Intn(5000)
	patch := rng.Intn(200)
	return fmt.Sprintf("Mozilla/5.0 (%s) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.%d.%d Safari/537.36", platform, major, build, patch)
}

func (a *App) queueStatusCountsLocked() (active, done, todo, failed int) {
	for _, job := range a.jobs {
		switch job.Status {
		case "running":
			active++
		case "completed", "stopped":
			done++
		case "failed":
			failed++
		default:
			todo++
		}
	}
	return active, done, todo, failed
}

func (a *App) logQueueStateLocked(event string, kv ...any) {
	active, done, todo, failed := a.queueStatusCountsLocked()
	fields := append([]any{}, kv...)
	fields = append(fields,
		"active_processing", active,
		"done", done,
		"to_do", todo,
		"failed", failed,
		"queue_depth", len(a.jobQueue),
		"queue_frozen", a.queueFrozen,
	)
	logEvent(event, fields...)
}

func (a *App) GetState() AppState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.snapshotLocked()
}

func (a *App) PickLocalFiles() ([]string, error) {
	if a.wailsApp == nil {
		return nil, errors.New("app not started")
	}
	return a.wailsApp.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:                   "Select music files to catalog",
		AllowsMultipleSelection: true,
		CanChooseFiles:          true,
		Filters: []application.FileFilter{
			{DisplayName: "Audio", Pattern: "*.mp3;*.m4a;*.aac;*.flac;*.wav;*.ogg;*.opus;*.webm"},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	}).PromptForMultipleSelection()
}

func (a *App) PickLocalFolder() (string, error) {
	if a.wailsApp == nil {
		return "", errors.New("app not started")
	}
	return a.wailsApp.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:                "Select a folder of music files to catalog",
		CanChooseDirectories: true,
		CanChooseFiles:       false,
	}).PromptForSingleSelection()
}

func (a *App) PickCookiesFile() (string, error) {
	if a.wailsApp == nil {
		return "", errors.New("app not started")
	}
	return a.wailsApp.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:          "Choose a yt-dlp cookies file",
		CanChooseFiles: true,
		Filters: []application.FileFilter{
			{DisplayName: "Cookies / text files", Pattern: "*.txt;*.cookie;*.cookies;*.netscape"},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	}).PromptForSingleSelection()
}

func (a *App) PickArtworkFile() (string, error) {
	if a.wailsApp == nil {
		return "", errors.New("app not started")
	}
	return a.wailsApp.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:          "Choose album art",
		CanChooseFiles: true,
		Filters: []application.FileFilter{
			{DisplayName: "Images", Pattern: "*.jpg;*.jpeg;*.png;*.webp;*.gif;*.bmp;*.avif"},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	}).PromptForSingleSelection()
}

func (a *App) ToggleWindowMaximise() error {
	if a.window == nil {
		return errors.New("app not started")
	}
	if a.window.IsFullscreen() {
		a.window.UnFullscreen()
	} else {
		a.window.Fullscreen()
	}
	return nil
}

func (a *App) LogUIEvent(event, name, from string) {
	if !envDebugEnabled(os.Getenv("MELODEX_DEBUG")) {
		return
	}
	event = strings.TrimSpace(event)
	name = strings.TrimSpace(name)
	from = strings.TrimSpace(from)
	if event == "" || name == "" {
		return
	}
	if from == "" {
		from = "Unknown"
	}
	logEvent(event, "name", name, "from", from)
}

func (a *App) LogVideoEvent(event, source, detail string) {
	if !envDebugEnabled(os.Getenv("MELODEX_DEBUG")) {
		return
	}
	event = strings.TrimSpace(event)
	source = strings.TrimSpace(source)
	detail = strings.TrimSpace(detail)
	if event == "" {
		return
	}
	fields := []any{}
	if source != "" {
		fields = append(fields, "source", source)
	}
	if detail != "" {
		fields = append(fields, "detail", detail)
	}
	logEvent(event, fields...)
}

func (a *App) PickLibraryRoot() (string, error) {
	if a.wailsApp == nil {
		return "", errors.New("app not started")
	}
	return a.wailsApp.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:                "Choose a Melodex Music directory",
		CanChooseDirectories: true,
		CanChooseFiles:       false,
	}).PromptForSingleSelection()
}

func (a *App) ApplyArtworkFromFile(trackID, sourcePath string) (AppState, error) {
	trackID = strings.TrimSpace(trackID)
	sourcePath = strings.TrimSpace(sourcePath)
	if trackID == "" {
		return AppState{}, errors.New("missing track id")
	}
	if sourcePath == "" {
		return AppState{}, errors.New("missing artwork file")
	}
	if !pathExists(sourcePath) {
		return AppState{}, fmt.Errorf("artwork file not found: %s", sourcePath)
	}

	ext := strings.ToLower(filepath.Ext(sourcePath))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".avif":
	default:
		ext = ".jpg"
	}

	a.mu.Lock()
	track, ok := a.trackIndex[trackID]
	if !ok {
		for _, candidate := range a.catalog.Tracks {
			if candidate.ID == trackID {
				track = candidate
				ok = true
				break
			}
		}
	}
	if !ok {
		a.mu.Unlock()
		return AppState{}, fmt.Errorf("track %s not found", trackID)
	}
	metadataPath := strings.TrimSpace(track.MetadataPath)
	if metadataPath == "" {
		metadataPath = strings.TrimSpace(track.BundlePath)
	}
	if metadataPath == "" {
		a.mu.Unlock()
		return AppState{}, fmt.Errorf("metadata path is not available for track %s", trackID)
	}
	albumDir := filepath.Dir(metadataPath)
	a.mu.Unlock()

	artworkPath := filepath.Join(albumDir, "cover"+ext)
	for _, stale := range []string{
		filepath.Join(albumDir, "cover.jpg"),
		filepath.Join(albumDir, "cover.jpeg"),
		filepath.Join(albumDir, "cover.png"),
		filepath.Join(albumDir, "cover.webp"),
		filepath.Join(albumDir, "cover.gif"),
		filepath.Join(albumDir, "cover.bmp"),
		filepath.Join(albumDir, "cover.avif"),
		filepath.Join(albumDir, "folder.jpg"),
	} {
		if filepath.Clean(stale) == filepath.Clean(artworkPath) {
			continue
		}
		_ = os.Remove(stale)
	}
	if filepath.Clean(sourcePath) != filepath.Clean(artworkPath) {
		if err := copyFile(sourcePath, artworkPath); err != nil {
			return AppState{}, err
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	albumDirClean := filepath.Clean(albumDir)
	now := time.Now().UTC()
	updated := 0
	for i := range a.catalog.Tracks {
		current := a.catalog.Tracks[i]
		currentMetaPath := strings.TrimSpace(current.MetadataPath)
		if currentMetaPath == "" {
			continue
		}
		if filepath.Clean(filepath.Dir(currentMetaPath)) != albumDirClean {
			continue
		}
		metadata, err := songstore.ReadMetadataJSON(currentMetaPath)
		if err != nil {
			metadata = metadataFromTrackRecord(current, currentMetaPath)
		} else {
			metadata = repairLoadedMetadata(currentMetaPath, metadata)
		}
		metadata = ensureMetadataSidecarPaths(current, metadata, currentMetaPath)
		metadata.Artwork = songstore.SongArtwork{
			Filename:  filepath.Base(artworkPath),
			Path:      artworkPath,
			Source:    "local-file",
			FetchedAt: now,
		}
		if err := songstore.WriteMetadataJSON(currentMetaPath, metadata); err != nil {
			return AppState{}, err
		}
		current.ArtworkPath = artworkPath
		current.ArtworkURL = ""
		current.ArtworkDataURL = ""
		current.ArtworkMediaURL = ""
		a.catalog.Tracks[i] = current
		updated++
	}
	if updated == 0 {
		return AppState{}, fmt.Errorf("no tracks found for album artwork update")
	}
	a.rebuildTrackIndexLocked()
	a.persistLocked()
	state := a.snapshotLocked()
	a.emitStateLocked(state)
	log.Printf("album artwork applied from %s to %s (%d tracks)", sourcePath, artworkPath, updated)
	logEvent("album_art_applied", "track_id", trackID, "source_path", sourcePath, "artwork_path", artworkPath, "track_count", updated)
	return state, nil
}

func (a *App) SaveSettings(input SettingsInput) (AppState, error) {
	if err := validateSettingsInput(input); err != nil {
		return AppState{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if input.LibraryRoot != "" {
		root, err := resolveRoot(input.LibraryRoot)
		if err != nil {
			logEvent("settings_save_failed", "operation", "resolve_library_root", "input_library_root", input.LibraryRoot, "error", err.Error())
			return AppState{}, err
		}
		store, info, err := newStore(root)
		if err != nil {
			logEvent("settings_save_failed", "operation", "open_store", "library_root", root, "error", err.Error())
			return AppState{}, err
		}
		var pipelineCache *PipelineCache
		if cache, cacheErr := loadPipelineCache(store.pipelineCachePath); cacheErr == nil {
			pipelineCache = cache
		} else {
			log.Printf("settings save: pipeline cache reload failed: %v", cacheErr)
			logEvent("pipeline_cache_load_failed", "operation", "settings_save", "error", cacheErr.Error())
			pipelineCache = &PipelineCache{path: store.pipelineCachePath}
		}
		a.store = store
		a.info = info
		a.catalog = catalogFile{Version: catalogSchemaVersion, Tracks: []TrackRecord{}, Jobs: a.jobs}
		playlists, _ := store.loadPlaylists()
		a.playlists = playlists.Playlists
		importHistory, _ := store.loadImportHistory()
		a.importHistory = importHistory.Entries
		a.trackIndex = map[string]TrackRecord{}
		a.player.setLookup(a.trackIndex)
		a.pipelineCache = pipelineCache
		a.settings.LibraryRoot = root
		if a.mediaServer != nil {
			a.mediaServer.setAllowedRoots([]string{
				a.info.LibraryRoot,
				a.info.AppDataDir,
				a.info.IncomingDir,
				a.info.CacheDir,
			})
		}
	}
	if input.AIBaseURL != "" {
		a.settings.AIBaseURL = input.AIBaseURL
	}
	if input.AIModel != "" {
		a.settings.AIModel = input.AIModel
	}
	if input.Provider != "" {
		a.settings.Provider = input.Provider
	}
	if input.APIKey != "" {
		a.settings.APIKey = input.APIKey
	}
	if input.YTDLPPath != "" {
		a.settings.YTDLPPath = input.YTDLPPath
	}
	if input.FFmpegPath != "" {
		a.settings.FFmpegPath = input.FFmpegPath
	}
	a.settings.YTDLPCookiesPath = input.YTDLPCookiesPath
	a.settings.YTDLPCookiesFromBrowser = input.YTDLPCookiesFromBrowser
	if strings.TrimSpace(input.VideoDownloadMode) != "" {
		a.settings.VideoDownloadMode = strings.ToLower(strings.TrimSpace(input.VideoDownloadMode))
	} else if input.DownloadMusicVideo {
		// Compatibility with v2 and older v3 clients that only send the bool.
		a.settings.VideoDownloadMode = videoDownloadModeDuringImport
	} else {
		// A legacy false value means video remains available on demand.
		a.settings.VideoDownloadMode = videoDownloadModeOnDemand
	}
	a.settings.DownloadMusicVideo = a.settings.VideoDownloadMode == videoDownloadModeDuringImport
	a.settings.KeepOriginalAudio = input.KeepOriginalAudio
	if input.MaxConcurrentDownloads > 0 {
		a.settings.MaxConcurrentDownloads = input.MaxConcurrentDownloads
	}
	if input.MaxConcurrentVideoDownloads > 0 {
		a.settings.MaxConcurrentVideoDownloads = input.MaxConcurrentVideoDownloads
	}
	if input.MaxConcurrentEnrichmentRequests > 0 {
		a.settings.MaxConcurrentEnrichmentRequests = input.MaxConcurrentEnrichmentRequests
	}
	if input.MaxConcurrentLyricsRequests > 0 {
		a.settings.MaxConcurrentLyricsRequests = input.MaxConcurrentLyricsRequests
	}
	a.settings.ThrottleOnYTDLPBotErrors = input.ThrottleOnYTDLPBotErrors
	a.settings.UpdateManifestURL = strings.TrimSpace(input.UpdateManifestURL)
	if err := a.store.saveSettings(a.settings); err != nil {
		logEvent("settings_save_failed", "operation", "save_settings", "library_root", a.settings.LibraryRoot, "error", err.Error())
		return AppState{}, err
	}
	_ = saveRootPreference(a.settings.LibraryRoot)
	a.ai = newAIClient(a.settings)
	a.aiStatus = aiStatusForSettings(a.settings, a.promptLibrary)
	a.toolReadinessCache = a.initialToolReadiness()
	a.updateInfo = defaultUpdateInfo(a.settings, a.buildInfo)
	if err := a.store.saveCatalog(a.catalog); err != nil {
		logEvent("settings_save_failed", "operation", "save_catalog", "library_root", a.settings.LibraryRoot, "error", err.Error())
		return AppState{}, err
	}
	if err := a.store.savePlaylists(playlistFile{Version: playlistSchemaVersion, Playlists: append([]Playlist(nil), a.playlists...)}); err != nil {
		logEvent("settings_save_failed", "operation", "save_playlists", "library_root", a.settings.LibraryRoot, "error", err.Error())
		return AppState{}, err
	}
	a.requestWorkerPoolReconcile()
	logEvent("settings_saved", "library_root", a.settings.LibraryRoot, "provider", a.settings.Provider, "ai_model", a.settings.AIModel, "download_music_video", a.settings.DownloadMusicVideo, "keep_original_audio", a.settings.KeepOriginalAudio, "cookies_from_browser", a.settings.YTDLPCookiesFromBrowser != "", "cookies_file", a.settings.YTDLPCookiesPath != "", "update_manifest", a.settings.UpdateManifestURL != "")
	state := a.snapshotLocked()
	a.emitStateLocked(state)
	return state, nil
}

func validateSettingsInput(input SettingsInput) error {
	if err := validateConcurrencySetting("max concurrent downloads", input.MaxConcurrentDownloads); err != nil {
		return err
	}
	if err := validateConcurrencySetting("max concurrent video downloads", input.MaxConcurrentVideoDownloads); err != nil {
		return err
	}
	if err := validateConcurrencySetting("max concurrent enrichment requests", input.MaxConcurrentEnrichmentRequests); err != nil {
		return err
	}
	if err := validateConcurrencySetting("max concurrent lyrics requests", input.MaxConcurrentLyricsRequests); err != nil {
		return err
	}
	if mode := strings.TrimSpace(input.VideoDownloadMode); mode != "" {
		switch strings.ToLower(mode) {
		case videoDownloadModeOff, videoDownloadModeOnDemand, videoDownloadModeDuringImport:
		default:
			return fmt.Errorf("video download mode must be %q, %q, or %q", videoDownloadModeOff, videoDownloadModeOnDemand, videoDownloadModeDuringImport)
		}
	}
	if strings.TrimSpace(input.YTDLPPath) != "" {
		if _, err := validateExecutablePath(input.YTDLPPath); err != nil {
			return fmt.Errorf("yt-dlp path: %w", err)
		}
	}
	if strings.TrimSpace(input.FFmpegPath) != "" {
		if _, err := validateExecutablePath(input.FFmpegPath); err != nil {
			return fmt.Errorf("ffmpeg path: %w", err)
		}
	}
	if strings.TrimSpace(input.YTDLPCookiesPath) != "" {
		if _, err := validateCookieFilePath(input.YTDLPCookiesPath); err != nil {
			return fmt.Errorf("cookies file: %w", err)
		}
	}
	if strings.TrimSpace(input.YTDLPCookiesFromBrowser) != "" {
		if _, err := validateBrowserCookieSelector(input.YTDLPCookiesFromBrowser); err != nil {
			return fmt.Errorf("cookies-from-browser: %w", err)
		}
	}
	if strings.TrimSpace(input.UpdateManifestURL) != "" {
		if _, err := validateHTTPURL(input.UpdateManifestURL); err != nil {
			return fmt.Errorf("update manifest url: %w", err)
		}
	}
	return nil
}

func validateConcurrencySetting(name string, value int) error {
	if value < 0 {
		return fmt.Errorf("%s cannot be negative", name)
	}
	if value > maxConfiguredConcurrency {
		return fmt.Errorf("%s cannot exceed %d", name, maxConfiguredConcurrency)
	}
	return nil
}

func aiStatusForSettings(settings storedSettings, prompts *PromptLibrary) string {
	if settings.APIKey == "" {
		return "Missing API key"
	}
	if prompts == nil {
		return "Prompt library unavailable"
	}
	return "Ready"
}

func (a *App) setAIStatus(message string) {
	a.mu.Lock()
	if a.aiStatus == message {
		a.mu.Unlock()
		return
	}
	a.aiStatus = message
	state := a.snapshotLocked()
	a.mu.Unlock()
	a.emitStateLocked(state)
}

func loadCachedLibraryCatalogSnapshot(store *Store) (catalogFile, error) {
	baseCatalog, err := store.loadCatalog()
	if err != nil {
		baseCatalog = catalogFile{Version: catalogSchemaVersion, Tracks: []TrackRecord{}, Jobs: []Job{}}
	}
	if cache, cacheErr := store.loadLibraryCache(); cacheErr == nil && len(cache.Tracks) > len(baseCatalog.Tracks) {
		baseCatalog = mergeCatalogFiles(baseCatalog, cache)
	}
	return baseCatalog, nil
}

func reconcileLibraryCatalogSnapshot(store *Store, root string, baseCatalog catalogFile) (catalogFile, error) {
	cached := make(map[string]TrackRecord, len(baseCatalog.Tracks))
	for _, track := range baseCatalog.Tracks {
		if track.MetadataPath != "" {
			cached[filepath.Clean(track.MetadataPath)] = track
		}
	}
	tracks, err := scanLibraryTracks(root, cached)
	if err != nil {
		return baseCatalog, err
	}
	if len(tracks) == 0 {
		return baseCatalog, nil
	}
	merged := mergeCatalogTracks(baseCatalog, tracks)
	if len(merged.Tracks) > 0 {
		_ = store.saveCatalog(merged)
		_ = store.saveLibraryCache(merged)
	}
	return merged, nil
}

func mergeCatalogFiles(base catalogFile, extra catalogFile) catalogFile {
	merged := catalogFile{
		Version: catalogSchemaVersion,
		Tracks:  append([]TrackRecord(nil), base.Tracks...),
		Jobs:    append([]Job(nil), base.Jobs...),
	}
	return mergeCatalogTracks(merged, extra.Tracks)
}

func mergeCatalogTracks(base catalogFile, tracks []TrackRecord) catalogFile {
	merged := catalogFile{
		Version: libraryCacheSchemaVersion,
		Tracks:  append([]TrackRecord(nil), base.Tracks...),
		Jobs:    append([]Job(nil), base.Jobs...),
	}
	seen := map[string]int{}
	for i, track := range merged.Tracks {
		if track.ID != "" {
			seen[track.ID] = i
		}
	}
	for _, track := range tracks {
		if track.ID == "" {
			continue
		}
		if idx, ok := seen[track.ID]; ok {
			merged.Tracks[idx] = track
			continue
		}
		seen[track.ID] = len(merged.Tracks)
		merged.Tracks = append(merged.Tracks, track)
	}
	sort.SliceStable(merged.Tracks, func(i, j int) bool {
		return merged.Tracks[i].CreatedAt.After(merged.Tracks[j].CreatedAt)
	})
	return merged
}

func scanLibraryTracks(root string, cached map[string]TrackRecord) ([]TrackRecord, error) {
	if root == "" {
		return []TrackRecord{}, nil
	}
	tracks := make([]TrackRecord, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") && base != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".metadata.json") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		cachedTrack, ok := cached[filepath.Clean(path)]
		if ok && cachedTrack.MetadataSize == info.Size() && cachedTrack.MetadataModTime.Equal(info.ModTime().UTC()) {
			tracks = append(tracks, cachedTrack)
			return nil
		}
		metadata, err := songstore.ReadMetadataJSON(path)
		if err != nil {
			return nil
		}
		metadata = repairLoadedMetadata(path, metadata)
		tracks = append(tracks, trackFromMetadataWithInfo(metadata, path, info))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(tracks, func(i, j int) bool {
		return tracks[i].CreatedAt.After(tracks[j].CreatedAt)
	})
	return tracks, nil
}

func repairLoadedMetadata(metadataPath string, metadata songstore.SongMetadata) songstore.SongMetadata {
	changed := false
	migrated := migrateTrackMetadata(metadata)
	if migrated.Version != metadata.Version || migrated.AI.Status != metadata.AI.Status || migrated.AI.Message != metadata.AI.Message || migrated.AI.Ran != metadata.AI.Ran {
		changed = true
	}
	metadata = migrated
	base := strings.TrimSuffix(filepath.Base(metadataPath), ".metadata.json")
	albumDir := filepath.Dir(metadataPath)
	if metadata.TrackID == "" {
		metadata.TrackID = songstore.TrackIDFromPath(metadataPath)
		changed = true
	}
	if metadata.StorageDir == "" {
		metadata.StorageDir = albumDir
		changed = true
	}
	if metadata.MetadataPath == "" {
		metadata.MetadataPath = metadataPath
		changed = true
	}
	if metadata.Audio.Path == "" {
		metadata.Audio.Path = filepath.Join(albumDir, base+".mp3")
		changed = true
	}
	if metadata.Audio.Filename == "" {
		metadata.Audio.Filename = base + ".mp3"
		changed = true
	}
	if metadata.Lyrics.Path == "" {
		metadata.Lyrics.Path = filepath.Join(albumDir, base+".txt")
		changed = true
	}
	if metadata.Lyrics.Filename == "" {
		metadata.Lyrics.Filename = base + ".txt"
		changed = true
	}
	if metadata.TimedLyrics.Path == "" {
		metadata.TimedLyrics.Path = filepath.Join(albumDir, base+".lrc")
		changed = true
	}
	if metadata.TimedLyrics.Filename == "" {
		metadata.TimedLyrics.Filename = base + ".lrc"
		changed = true
	}
	if metadata.Artwork.Path == "" && metadata.Artwork.URL != "" {
		metadata.Artwork.Path = filepath.Join(albumDir, "cover.jpg")
		metadata.Artwork.Filename = filepath.Base(metadata.Artwork.Path)
		changed = true
	}
	if metadata.Artwork.Path != "" && metadata.Artwork.Filename == "" {
		metadata.Artwork.Filename = filepath.Base(metadata.Artwork.Path)
		changed = true
	}
	if metadata.Video.Path != "" && metadata.Video.Filename == "" {
		metadata.Video.Filename = filepath.Base(metadata.Video.Path)
		changed = true
	}
	if metadata.Video.Path == "" && (metadata.Video.Source != "" || metadata.Video.URL != "") {
		metadata.Video.Path = filepath.Join(albumDir, base+".mp4")
		metadata.Video.Filename = filepath.Base(metadata.Video.Path)
		changed = true
	}
	if normalized, aiChanged := songstore.EnsureAIStatus(metadata); aiChanged {
		metadata = normalized
		changed = true
	}
	if changed {
		_ = songstore.WriteMetadataJSON(metadataPath, metadata)
	}
	return metadata
}

func repairTrackFiles(track TrackRecord) error {
	metadataPath := strings.TrimSpace(track.MetadataPath)
	if metadataPath == "" {
		metadataPath = strings.TrimSpace(track.BundlePath)
	}
	if metadataPath == "" {
		return nil
	}
	metadata, err := songstore.ReadMetadataJSON(metadataPath)
	if err != nil {
		metadata = metadataFromTrackRecord(track, metadataPath)
	} else {
		metadata = repairLoadedMetadata(metadataPath, metadata)
	}
	metadata = ensureMetadataSidecarPaths(track, metadata, metadataPath)
	if metadata.Lyrics.Path != "" && !pathExists(metadata.Lyrics.Path) {
		_ = songstore.WritePlainLyrics(metadata.Lyrics.Path, "")
	}
	if metadata.TimedLyrics.Path != "" && !pathExists(metadata.TimedLyrics.Path) {
		_ = songstore.WriteLRC(metadata.TimedLyrics.Path, nil)
	}
	return songstore.WriteMetadataJSON(metadataPath, metadata)
}

func metadataFromTrackRecord(track TrackRecord, metadataPath string) songstore.SongMetadata {
	aiContext := track.AIContext
	if !hasMeaningfulAIContext(aiContext) {
		aiContext = songAIContextFromTrackRecord(track, metadataPath)
	}
	metadataConfidence := track.MetadataConfidence
	if strings.TrimSpace(metadataConfidence) == "" {
		metadataConfidence = track.Confidence
	}
	overallConfidence := track.Confidence
	if strings.TrimSpace(overallConfidence) == "" {
		overallConfidence = metadataConfidence
	}
	return songstore.SongMetadata{
		Version:              trackMetadataSchemaVersion,
		TrackID:              track.ID,
		Title:                cleanDisplayName(track.Title),
		Artist:               cleanDisplayName(track.Artist),
		Album:                cleanDisplayName(track.Album),
		Year:                 parseOptionalYear(track.Year),
		Genre:                cleanDisplayName(track.Genre),
		Confidence:           overallConfidence,
		MetadataConfidence:   metadataConfidence,
		EnrichmentConfidence: track.EnrichmentConfidence,
		ReleaseType:          track.ReleaseType,
		ISRC:                 track.ISRC,
		ArtistLinks:          metadataLinksToStore(track.ArtistLinks),
		AlbumLinks:           metadataLinksToStore(track.AlbumLinks),
		SongLinks:            metadataLinksToStore(track.SongLinks),
		ArtistTrivia:         append([]string(nil), track.ArtistTrivia...),
		AlbumTrivia:          append([]string(nil), track.AlbumTrivia...),
		SongTrivia:           append([]string(nil), track.SongTrivia...),
		SongMeaning:          track.SongMeaning,
		Tidbits:              append([]string(nil), track.Tidbits...),
		Sources:              append([]string(nil), track.Sources...),
		MetadataNotes:        track.MetadataNotes,
		Source: songstore.SongSource{
			URL:          track.SourceRef,
			Provider:     track.SourceKind,
			VideoTitle:   track.SourceTitle,
			Channel:      track.SourceChannel,
			DownloadedAt: track.GeneratedAt,
		},
		Audio: songstore.SongAudio{
			Filename: baseNameOrEmpty(track.AudioPath),
			Format:   "mp3",
			Path:     track.AudioPath,
		},
		Lyrics: songstore.SongLyrics{
			Filename:   baseNameOrEmpty(track.LyricsPath),
			Path:       track.LyricsPath,
			HasLyrics:  track.LyricsIncluded,
			IsComplete: track.LyricsIncluded,
			Confidence: track.LyricsConfidence,
			Source:     track.LyricsSource,
			SourceLoc:  track.LyricsSourceLoc,
			Notes:      track.MetadataNotes,
		},
		TimedLyrics: songstore.SongTimedLyrics{
			Filename:       baseNameOrEmpty(track.LRCPath),
			Path:           track.LRCPath,
			HasTimedLyrics: track.HasTimedLyrics,
			Format:         "lrc",
			Granularity:    track.TimedLyricsGranularity,
			Confidence:     track.TimedLyricsConfidence,
			Source:         track.LyricsSource,
		},
		Artwork: songstore.SongArtwork{
			Filename:       baseNameOrEmpty(track.ArtworkPath),
			Path:           track.ArtworkPath,
			URL:            track.ArtworkURL,
			ReleaseID:      "",
			ReleaseGroupID: "",
		},
		Video: songstore.SongVideo{
			Filename:  baseNameOrEmpty(track.VideoPath),
			Path:      track.VideoPath,
			Source:    track.SourceKind,
			URL:       track.VideoURL,
			FetchedAt: track.GeneratedAt,
		},
		AI: songstore.SongAI{
			Provider:    track.AIProvider,
			Model:       track.AIModel,
			Ran:         track.AIRan,
			Status:      track.AIStatus,
			Message:     track.AIMessage,
			Context:     aiContext,
			GeneratedAt: track.GeneratedAt,
		},
		StorageDir:   filepath.Dir(metadataPath),
		MetadataPath: metadataPath,
	}
}

func hasMeaningfulAIContext(context SongAIContext) bool {
	return strings.TrimSpace(context.SourceURL) != "" ||
		strings.TrimSpace(context.LibraryRoot) != "" ||
		strings.TrimSpace(context.TargetMetadataPath) != "" ||
		strings.TrimSpace(context.ResolvedTitle) != "" ||
		strings.TrimSpace(context.ResolvedArtist) != "" ||
		strings.TrimSpace(context.UserContext) != ""
}

func songAIContextFromTrackRecord(track TrackRecord, metadataPath string) songstore.SongAIContext {
	albumDir := filepath.Dir(metadataPath)
	artistDir := filepath.Dir(albumDir)
	libraryRoot := filepath.Dir(artistDir)
	fileName := baseNameOrEmpty(track.AudioPath)
	baseName := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	if baseName == "" {
		baseName = strings.TrimSuffix(filepath.Base(metadataPath), ".metadata.json")
	}
	return songstore.SongAIContext{
		FileName:              fileName,
		SourceRef:             track.SourceRef,
		SourceURL:             track.SourceRef,
		SourceKind:            track.SourceKind,
		SourceTitle:           track.SourceTitle,
		SourceChannel:         track.SourceChannel,
		LibraryRoot:           libraryRoot,
		ResolvedTitle:         track.Title,
		ResolvedArtist:        track.Artist,
		ResolvedAlbum:         track.Album,
		ResolvedConfidence:    firstTrackConfidence(track),
		ResolvedNotes:         track.MetadataNotes,
		TargetArtistDir:       artistDir,
		TargetAlbumDir:        albumDir,
		TargetBaseName:        baseName,
		TargetAudioPath:       track.AudioPath,
		TargetLyricsPath:      track.LyricsPath,
		TargetTimedLyricsPath: track.LRCPath,
		TargetMetadataPath:    metadataPath,
		UserContext:           track.AIContext.UserContext,
	}
}

func firstTrackConfidence(track TrackRecord) string {
	if strings.TrimSpace(track.MetadataConfidence) != "" {
		return track.MetadataConfidence
	}
	return track.Confidence
}

func ensureMetadataSidecarPaths(track TrackRecord, metadata songstore.SongMetadata, metadataPath string) songstore.SongMetadata {
	albumDir := filepath.Dir(metadataPath)
	if metadata.TrackID == "" {
		metadata.TrackID = track.ID
	}
	if metadata.StorageDir == "" {
		metadata.StorageDir = albumDir
	}
	if metadata.MetadataPath == "" {
		metadata.MetadataPath = metadataPath
	}
	if metadata.Audio.Path == "" {
		metadata.Audio.Path = track.AudioPath
	}
	if metadata.Audio.Filename == "" && track.AudioPath != "" {
		metadata.Audio.Filename = baseNameOrEmpty(track.AudioPath)
	}
	if metadata.Lyrics.Path == "" {
		metadata.Lyrics.Path = track.LyricsPath
	}
	if metadata.Lyrics.Filename == "" && track.LyricsPath != "" {
		metadata.Lyrics.Filename = baseNameOrEmpty(track.LyricsPath)
	}
	if metadata.TimedLyrics.Path == "" {
		metadata.TimedLyrics.Path = track.LRCPath
	}
	if metadata.TimedLyrics.Filename == "" && track.LRCPath != "" {
		metadata.TimedLyrics.Filename = baseNameOrEmpty(track.LRCPath)
	}
	if metadata.Artwork.Path == "" {
		metadata.Artwork.Path = track.ArtworkPath
	}
	if metadata.Artwork.Filename == "" && metadata.Artwork.Path != "" {
		metadata.Artwork.Filename = baseNameOrEmpty(metadata.Artwork.Path)
	}
	if metadata.Video.Path == "" {
		metadata.Video.Path = track.VideoPath
	}
	if metadata.Video.URL == "" {
		metadata.Video.URL = track.VideoURL
	}
	if metadata.Video.Filename == "" && metadata.Video.Path != "" {
		metadata.Video.Filename = baseNameOrEmpty(metadata.Video.Path)
	}
	return metadata
}

func baseNameOrEmpty(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return filepath.Base(path)
}

func parseOptionalYear(value string) *int {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	year, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	return &year
}

func pathExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func isPermissionDeniedError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return os.IsPermission(err) || strings.Contains(lower, "operation not permitted") || strings.Contains(lower, "permission denied")
}

func (a *App) reconcileLibraryFromDisk() {
	store := a.store
	libraryRoot := a.info.LibraryRoot
	if store == nil || libraryRoot == "" {
		return
	}
	log.Printf("startup: reconciling catalog from disk")
	logEvent("startup_reconcile_started", "library_root", libraryRoot)
	baseCatalog, err := loadCachedLibraryCatalogSnapshot(store)
	if err != nil {
		baseCatalog = catalogFile{Version: catalogSchemaVersion, Tracks: []TrackRecord{}, Jobs: []Job{}}
	}
	merged, err := reconcileLibraryCatalogSnapshot(store, libraryRoot, baseCatalog)
	if err != nil {
		if isPermissionDeniedError(err) {
			log.Printf("startup: library root access denied: %v", err)
			logEvent("startup_reconcile_access_denied", "library_root", libraryRoot, "error", err.Error())
			time.Sleep(1500 * time.Millisecond)
			merged, err = reconcileLibraryCatalogSnapshot(store, libraryRoot, baseCatalog)
			if err == nil {
				logEvent("startup_reconcile_access_restored", "library_root", libraryRoot)
			} else if isPermissionDeniedError(err) {
				logEvent("startup_reconcile_permission_required", "library_root", libraryRoot, "error", err.Error())
				return
			} else {
				logEvent("startup_reconcile_failed", "library_root", libraryRoot, "error", err.Error())
				return
			}
		} else {
			logEvent("startup_reconcile_failed", "library_root", libraryRoot, "error", err.Error())
			return
		}
	}
	a.mu.Lock()
	if a.store != store || a.info.LibraryRoot != libraryRoot {
		a.mu.Unlock()
		logEvent("startup_reconcile_skipped", "library_root", libraryRoot, "reason", "library_root_changed")
		return
	}
	a.catalog = merged
	a.rebuildTrackIndexLocked()
	state := a.snapshotLocked()
	a.mu.Unlock()
	a.emitStateLocked(state)
	logEvent("startup_reconcile_completed", "library_root", libraryRoot, "track_count", len(merged.Tracks))
}

func (a *App) ClearAPIKey() (AppState, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.settings.APIKey = ""
	if err := a.store.saveSettings(a.settings); err != nil {
		logEvent("settings_save_failed", "operation", "clear_api_key", "library_root", a.settings.LibraryRoot, "error", err.Error())
		return AppState{}, err
	}
	a.ai = newAIClient(a.settings)
	a.aiStatus = aiStatusForSettings(a.settings, a.promptLibrary)
	logEvent("settings_api_key_cleared", "library_root", a.settings.LibraryRoot)
	state := a.snapshotLocked()
	a.emitStateLocked(state)
	return state, nil
}

func (a *App) ClearImportHistory() (AppState, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.importHistory = []ImportHistoryEntry{}
	a.persistLocked()
	state := a.snapshotLocked()
	a.emitStateLocked(state)
	return state, nil
}

func (a *App) recordImportHistory(url string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	url = strings.TrimSpace(url)
	if url == "" {
		return
	}
	a.importHistory = append([]ImportHistoryEntry{{
		URL:         url,
		CompletedAt: time.Now().UTC(),
	}}, a.importHistory...)
	a.persistLocked()
	a.emitStateLocked(a.snapshotLocked())
}

func (a *App) QueueLocalFiles(paths []string) ([]Job, error) {
	if len(paths) == 0 {
		logEvent("import_queue_failed", "kind", "local_file", "reason", "no_files_selected")
		return nil, errors.New("no files selected")
	}
	files, err := collectLocalImportFiles(paths)
	if err != nil {
		logEvent("import_queue_failed", "kind", "local_file", "reason", err.Error(), "selected_count", len(paths))
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	jobs := make([]Job, 0, len(files))
	for _, file := range files {
		job := a.addJobLocked("local-file", file.Path, file.SourceRoot)
		jobs = append(jobs, job)
	}
	logEvent("import_queued", "kind", "local_file", "selected_count", len(paths), "job_count", len(jobs), "library_root", a.settings.LibraryRoot)
	a.persistLocked()
	a.emitStateLocked(a.snapshotLocked())
	return jobs, nil
}

func (a *App) QueueURLImport(url string) (Job, error) {
	normalized, err := validateHTTPURL(url)
	if err != nil {
		logEvent("import_queue_failed", "kind", "url", "reason", err.Error())
		return Job{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	job := a.addJobLocked("url", normalized, "")
	logEvent("import_queued", "kind", "url", "source_url", normalized)
	a.persistLocked()
	a.emitStateLocked(a.snapshotLocked())
	return job, nil
}

func (a *App) FindURLImportDuplicate(url string) (URLImportDuplicateInfo, error) {
	normalized, err := validateHTTPURL(url)
	if err != nil {
		return URLImportDuplicateInfo{}, err
	}
	keys := canonicalURLDuplicateKeys(normalized)
	if len(keys) == 0 {
		return URLImportDuplicateInfo{}, nil
	}
	keySet := map[string]struct{}{}
	for _, key := range keys {
		keySet[key] = struct{}{}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	for _, track := range a.catalog.Tracks {
		trackKeys := append(canonicalURLDuplicateKeys(track.SourceRef), canonicalURLDuplicateKeys(track.VideoURL)...)
		for _, key := range trackKeys {
			if _, ok := keySet[key]; !ok {
				continue
			}
			reason := strings.TrimSpace(key)
			if strings.HasPrefix(key, "youtube:video:") {
				reason = "matched video ID"
			} else if strings.HasPrefix(key, "youtube:playlist:") {
				reason = "matched playlist ID"
			} else if strings.HasPrefix(key, "exact:") {
				reason = "matched exact source URL"
			}
			return URLImportDuplicateInfo{
				Exists:       true,
				TrackID:      track.ID,
				MetadataPath: track.MetadataPath,
				Title:        track.Title,
				Artist:       track.Artist,
				Album:        track.Album,
				SourceURL:    firstNonEmpty(track.SourceRef, track.VideoURL),
				VideoURL:     track.VideoURL,
				MatchReason:  reason,
			}, nil
		}
	}
	for _, job := range a.jobs {
		trackKeys := append(canonicalURLDuplicateKeys(job.Input), canonicalURLDuplicateKeys(job.PlaylistURL)...)
		for _, key := range trackKeys {
			if _, ok := keySet[key]; !ok {
				continue
			}
			reason := strings.TrimSpace(key)
			if strings.HasPrefix(key, "youtube:video:") {
				reason = "matched queued video ID"
			} else if strings.HasPrefix(key, "youtube:playlist:") {
				reason = "matched queued playlist ID"
			} else if strings.HasPrefix(key, "exact:") {
				reason = "matched queued exact URL"
			}
			return URLImportDuplicateInfo{
				Exists:      true,
				JobID:       job.ID,
				Title:       firstNonEmpty(job.ResultTitle, job.PlaylistItemTitle, job.PlaylistTitle, job.Input),
				Artist:      firstNonEmpty(job.ResultArtist, job.PlaylistChannel),
				Album:       firstNonEmpty(job.ResultAlbum, job.PlaylistTitle),
				SourceURL:   firstNonEmpty(job.Input, job.PlaylistURL),
				MatchReason: reason,
			}, nil
		}
	}
	return URLImportDuplicateInfo{}, nil
}

func (a *App) ReprocessTrackWithContext(trackID, userContext string) (AppState, error) {
	trackID = strings.TrimSpace(trackID)
	userContext = strings.TrimSpace(userContext)
	if trackID == "" {
		logEvent("track_reprocess_failed", "reason", "missing_track_id")
		return AppState{}, errors.New("track id is required")
	}
	if userContext == "" {
		logEvent("track_reprocess_failed", "track_id", trackID, "reason", "missing_context")
		return AppState{}, errors.New("context is required")
	}
	if a.ai == nil || !a.ai.available() {
		logEvent("track_reprocess_failed", "track_id", trackID, "reason", "missing_api_key")
		return AppState{}, errors.New("AI key is required to reprocess with added context")
	}

	a.mu.Lock()
	track, ok := a.trackIndex[trackID]
	if !ok {
		for _, candidate := range a.catalog.Tracks {
			if candidate.ID == trackID {
				track = candidate
				ok = true
				break
			}
		}
	}
	if !ok {
		a.mu.Unlock()
		logEvent("track_reprocess_failed", "track_id", trackID, "reason", "track_not_found")
		return AppState{}, fmt.Errorf("track %s not found", trackID)
	}
	label := strings.TrimSpace(track.Title)
	if track.Artist != "" {
		label = fmt.Sprintf("%s - %s", track.Artist, track.Title)
	}
	job := Job{
		Kind:         "reprocess-track",
		Input:        label,
		TrackID:      track.ID,
		UserContext:  userContext,
		Status:       "queued",
		Detail:       "Waiting for worker",
		ResultTitle:  track.Title,
		ResultArtist: track.Artist,
		ResultAlbum:  track.Album,
		CreatedAt:    time.Now().UTC(),
	}
	job = a.addReprocessJobLocked(job)
	logEvent("track_reprocess_queued", "job_id", job.ID, "track_id", track.ID, "track_title", track.Title)
	a.persistLocked()
	state := a.snapshotLocked()
	a.mu.Unlock()
	a.emitStateLocked(state)
	return state, nil
}

func (a *App) RescanLibrary() (AppState, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	logEvent("library_rescan_started", "library_root", a.info.LibraryRoot)
	baseCatalog, err := loadCachedLibraryCatalogSnapshot(a.store)
	if err != nil {
		logEvent("library_rescan_warning", "operation", "load_cached_catalog", "error", err.Error())
		return AppState{}, err
	}
	catalog, err := reconcileLibraryCatalogSnapshot(a.store, a.info.LibraryRoot, baseCatalog)
	if err != nil {
		logEvent("library_rescan_failed", "library_root", a.info.LibraryRoot, "error", err.Error())
		return AppState{}, err
	}
	a.catalog = catalog
	a.rebuildTrackIndexLocked()
	logEvent("library_rescan_completed", "library_root", a.info.LibraryRoot, "track_count", len(a.catalog.Tracks))
	state := a.snapshotLocked()
	a.emitStateLocked(state)
	return state, nil
}

func (a *App) RepairLibraryFiles() (AppState, error) {
	logEvent("library_repair_started")
	a.mu.Lock()
	tracks := append([]TrackRecord(nil), a.catalog.Tracks...)
	a.mu.Unlock()
	for _, track := range tracks {
		_ = repairTrackFiles(track)
	}
	state, err := a.RescanLibrary()
	if err == nil {
		logEvent("library_repair_completed", "track_count", len(tracks))
	}
	return state, err
}

func (a *App) ReprocessUnprocessedTracks() (AppState, error) {
	a.mu.Lock()
	tracks := append([]TrackRecord(nil), a.catalog.Tracks...)
	a.mu.Unlock()
	paths := make([]string, 0)
	for _, track := range tracks {
		if !isTrackUnprocessed(track) || strings.TrimSpace(track.AudioPath) == "" {
			continue
		}
		paths = append(paths, track.AudioPath)
	}
	if len(paths) == 0 {
		logEvent("reprocess_unprocessed_skipped", "reason", "no_unprocessed_tracks")
		return a.GetState(), nil
	}
	logEvent("reprocess_unprocessed_started", "track_count", len(paths))
	if _, err := a.QueueLocalFiles(paths); err != nil {
		logEvent("reprocess_unprocessed_failed", "track_count", len(paths), "error", err.Error())
		return AppState{}, err
	}
	logEvent("reprocess_unprocessed_queued", "track_count", len(paths))
	return a.GetState(), nil
}

func (a *App) InspectBundle(path string) (TrackRecord, error) {
	if _, err := validatePathWithinRoots(path, []string{a.info.LibraryRoot, a.info.AppDataDir, a.info.IncomingDir, a.info.CacheDir}, false); err != nil {
		return TrackRecord{}, fmt.Errorf("bundle path: %w", err)
	}
	metadata, err := songstore.LoadSongMetadata(path)
	if err != nil {
		return TrackRecord{}, err
	}
	metadataPath := metadata.MetadataPath
	if metadataPath == "" {
		metadataPath = path
	}
	return a.trackWithArtworkMediaURL(trackFromMetadata(metadata, metadataPath)), nil
}

func (a *App) InspectTrack(metadataPath string) (TrackPreview, error) {
	if _, err := validatePathWithinRoots(metadataPath, []string{a.info.LibraryRoot, a.info.AppDataDir, a.info.IncomingDir, a.info.CacheDir}, false); err != nil {
		return TrackPreview{}, fmt.Errorf("metadata path: %w", err)
	}
	metadata, err := songstore.LoadSongMetadata(metadataPath)
	if err != nil {
		return TrackPreview{}, err
	}
	track := a.trackWithArtworkMediaURL(trackFromMetadata(metadata, metadataPath))
	metadataConfidence := metadata.MetadataConfidence
	if strings.TrimSpace(metadataConfidence) == "" {
		metadataConfidence = metadata.Confidence
	}
	overallConfidence := metadata.Confidence
	if strings.TrimSpace(overallConfidence) == "" {
		overallConfidence = metadataConfidence
	}
	lyricsText := ""
	if metadata.Lyrics.Path != "" {
		if data, err := os.ReadFile(metadata.Lyrics.Path); err == nil {
			lyricsText = string(data)
		}
	}
	timedLyricsText := ""
	if metadata.TimedLyrics.Path != "" {
		if data, err := os.ReadFile(metadata.TimedLyrics.Path); err == nil {
			timedLyricsText = string(data)
		}
	}
	audioLabel := strings.ToUpper(audioFormatFromPath(metadata.Audio.Path))
	if audioLabel == "" {
		audioLabel = "AUDIO"
	}
	files := []TrackFileState{
		fileState(audioLabel, metadata.Audio.Path),
		fileState("TXT", metadata.Lyrics.Path),
		fileState("LRC", metadata.TimedLyrics.Path),
		fileState("MP4", metadata.Video.Path),
		fileState("metadata.json", metadataPath),
	}
	return TrackPreview{
		Track:                  track,
		LyricsText:             lyricsText,
		TimedLyricsText:        timedLyricsText,
		FileStates:             files,
		Title:                  metadata.Title,
		Artist:                 metadata.Artist,
		Album:                  metadata.Album,
		TrackNumber:            metadata.TrackNumber,
		Year:                   metadata.Year,
		Genre:                  metadata.Genre,
		DurationSeconds:        metadata.DurationSeconds,
		Confidence:             overallConfidence,
		MetadataConfidence:     metadataConfidence,
		EnrichmentConfidence:   metadata.EnrichmentConfidence,
		ReleaseType:            metadata.ReleaseType,
		ISRC:                   metadata.ISRC,
		ArtistLinks:            metadataLinksFromStore(metadata.ArtistLinks),
		AlbumLinks:             metadataLinksFromStore(metadata.AlbumLinks),
		SongLinks:              metadataLinksFromStore(metadata.SongLinks),
		ArtistTrivia:           append([]string(nil), metadata.ArtistTrivia...),
		AlbumTrivia:            append([]string(nil), metadata.AlbumTrivia...),
		SongTrivia:             append([]string(nil), metadata.SongTrivia...),
		SongMeaning:            metadata.SongMeaning,
		Tidbits:                append([]string(nil), metadata.Tidbits...),
		Sources:                append([]string(nil), metadata.Sources...),
		MetadataNotes:          metadata.MetadataNotes,
		SourceURL:              metadata.Source.URL,
		SourceProvider:         metadata.Source.Provider,
		SourceVideoID:          metadata.Source.VideoID,
		SourceTitle:            metadata.Source.VideoTitle,
		SourceChannel:          metadata.Source.Channel,
		AIProvider:             metadata.AI.Provider,
		AIModel:                metadata.AI.Model,
		AIRan:                  metadata.AI.Ran,
		AIStatus:               metadata.AI.Status,
		AIMessage:              metadata.AI.Message,
		AIContext:              metadata.AI.Context,
		GeneratedAt:            metadata.AI.GeneratedAt,
		LyricsConfidence:       metadata.Lyrics.Confidence,
		LyricsSource:           metadata.Lyrics.Source,
		LyricsSourceLoc:        metadata.Lyrics.SourceLoc,
		TimedLyricsConfidence:  metadata.TimedLyrics.Confidence,
		TimedLyricsSource:      metadata.TimedLyrics.Source,
		TimedLyricsGranularity: metadata.TimedLyrics.Granularity,
		ArtworkPath:            track.ArtworkPath,
		ArtworkDataURL:         track.ArtworkDataURL,
		ArtworkMediaURL:        track.ArtworkMediaURL,
		VideoPath:              track.VideoPath,
		VideoURL:               track.VideoURL,
	}, nil
}

func trackFromMetadata(metadata songstore.SongMetadata, metadataPath string) TrackRecord {
	return trackFromMetadataWithInfo(metadata, metadataPath, nil)
}

func trackFromMetadataWithInfo(metadata songstore.SongMetadata, metadataPath string, info os.FileInfo) TrackRecord {
	trackID := metadata.TrackID
	if trackID == "" {
		trackID = songstore.TrackIDFromPath(metadataPath)
	}
	year := ""
	if metadata.Year != nil {
		year = fmt.Sprintf("%d", *metadata.Year)
	}
	lyricsIncluded := metadata.Lyrics.HasLyrics
	metadataConfidence := metadata.MetadataConfidence
	if strings.TrimSpace(metadataConfidence) == "" {
		metadataConfidence = metadata.Confidence
	}
	overallConfidence := metadata.Confidence
	if strings.TrimSpace(overallConfidence) == "" {
		overallConfidence = metadataConfidence
	}
	bundlePath := metadata.MetadataPath
	if bundlePath == "" {
		bundlePath = metadataPath
	}
	storageDir := metadata.StorageDir
	if storageDir == "" {
		storageDir = filepath.Dir(metadataPath)
	}
	track := TrackRecord{
		ID:                     trackID,
		Artist:                 metadata.Artist,
		Album:                  metadata.Album,
		Title:                  metadata.Title,
		Genre:                  metadata.Genre,
		Year:                   year,
		LyricsIncluded:         lyricsIncluded,
		Confidence:             overallConfidence,
		BundlePath:             bundlePath,
		StorageDir:             storageDir,
		AudioPath:              metadata.Audio.Path,
		LyricsPath:             metadata.Lyrics.Path,
		LRCPath:                metadata.TimedLyrics.Path,
		ArtworkPath:            metadata.Artwork.Path,
		ArtworkURL:             metadata.Artwork.URL,
		VideoPath:              metadata.Video.Path,
		VideoURL:               metadata.Video.URL,
		MetadataPath:           metadataPath,
		MetadataConfidence:     metadataConfidence,
		EnrichmentConfidence:   metadata.EnrichmentConfidence,
		ReleaseType:            metadata.ReleaseType,
		ISRC:                   metadata.ISRC,
		ArtistLinks:            metadataLinksFromStore(metadata.ArtistLinks),
		AlbumLinks:             metadataLinksFromStore(metadata.AlbumLinks),
		SongLinks:              metadataLinksFromStore(metadata.SongLinks),
		ArtistTrivia:           append([]string(nil), metadata.ArtistTrivia...),
		AlbumTrivia:            append([]string(nil), metadata.AlbumTrivia...),
		SongTrivia:             append([]string(nil), metadata.SongTrivia...),
		SongMeaning:            metadata.SongMeaning,
		Tidbits:                append([]string(nil), metadata.Tidbits...),
		Sources:                append([]string(nil), metadata.Sources...),
		MetadataNotes:          metadata.MetadataNotes,
		LyricsConfidence:       metadata.Lyrics.Confidence,
		LyricsSource:           metadata.Lyrics.Source,
		LyricsSourceLoc:        metadata.Lyrics.SourceLoc,
		TimedLyricsConfidence:  metadata.TimedLyrics.Confidence,
		TimedLyricsGranularity: metadata.TimedLyrics.Granularity,
		HasTimedLyrics:         metadata.TimedLyrics.HasTimedLyrics,
		SourceTitle:            metadata.Source.VideoTitle,
		SourceChannel:          metadata.Source.Channel,
		AIProvider:             metadata.AI.Provider,
		AIModel:                metadata.AI.Model,
		AIRan:                  metadata.AI.Ran,
		AIStatus:               metadata.AI.Status,
		AIMessage:              metadata.AI.Message,
		AIContext:              metadata.AI.Context,
		GeneratedAt:            metadata.AI.GeneratedAt,
		SourceKind:             metadata.Source.Provider,
		SourceRef:              metadata.Source.URL,
		DurationSeconds:        metadata.DurationSeconds,
		MetadataSize:           fileSize(info),
		MetadataModTime:        fileModTime(info),
		Hash:                   "",
		CreatedAt:              metadata.AI.GeneratedAt,
	}
	return track
}

func metadataLinksFromStore(links songstore.MetadataLinks) MetadataLinks {
	return MetadataLinks{
		OfficialWebsite: links.OfficialWebsite,
		Spotify:         links.Spotify,
		AppleMusic:      links.AppleMusic,
		YouTube:         links.YouTube,
		YouTubeMusic:    links.YouTubeMusic,
		Instagram:       links.Instagram,
		X:               links.X,
		Facebook:        links.Facebook,
		Bandcamp:        links.Bandcamp,
		SoundCloud:      links.SoundCloud,
		Wikipedia:       links.Wikipedia,
		MusicBrainz:     links.MusicBrainz,
		Genius:          links.Genius,
	}
}

func metadataLinksToStore(links MetadataLinks) songstore.MetadataLinks {
	return songstore.MetadataLinks{
		OfficialWebsite: links.OfficialWebsite,
		Spotify:         links.Spotify,
		AppleMusic:      links.AppleMusic,
		YouTube:         links.YouTube,
		YouTubeMusic:    links.YouTubeMusic,
		Instagram:       links.Instagram,
		X:               links.X,
		Facebook:        links.Facebook,
		Bandcamp:        links.Bandcamp,
		SoundCloud:      links.SoundCloud,
		Wikipedia:       links.Wikipedia,
		MusicBrainz:     links.MusicBrainz,
		Genius:          links.Genius,
	}
}

func (a *App) ReadFileDataURL(path string) (string, error) {
	if err := a.ensureMediaServer(); err != nil {
		return "", err
	}
	a.mediaMu.Lock()
	server := a.mediaServer
	a.mediaMu.Unlock()
	if server == nil {
		return "", errors.New("media server unavailable")
	}
	return server.ReadDataURL(path)
}

func (a *App) artworkMediaURLForPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	a.mediaMu.Lock()
	server := a.mediaServer
	a.mediaMu.Unlock()
	if server == nil {
		if err := a.ensureMediaServer(); err != nil {
			return ""
		}
		a.mediaMu.Lock()
		server = a.mediaServer
		a.mediaMu.Unlock()
	}
	if server == nil {
		return ""
	}
	url, _, err := server.URLFor(trimmed)
	if err != nil {
		return ""
	}
	return url
}

func (a *App) trackWithArtworkMediaURL(track TrackRecord) TrackRecord {
	if resolved := resolveTrackArtworkPath(track); resolved != "" {
		track.ArtworkPath = resolved
		// ArtworkMediaURL is the canonical streamed artwork field.
		// ArtworkDataURL remains populated for compatibility while older UI paths migrate.
		track.ArtworkDataURL = a.artworkMediaURLForPath(resolved)
	} else if strings.TrimSpace(track.ArtworkURL) != "" {
		track.ArtworkDataURL = track.ArtworkURL
	}
	track.ArtworkMediaURL = track.ArtworkDataURL
	if resolved := resolveTrackVideoPath(track); resolved != "" {
		track.VideoPath = resolved
	}
	return track
}

func resolveTrackArtworkPath(track TrackRecord) string {
	candidates := []string{track.ArtworkPath}
	if strings.TrimSpace(track.StorageDir) != "" {
		candidates = append(candidates,
			filepath.Join(track.StorageDir, "cover.jpg"),
			filepath.Join(track.StorageDir, "cover.png"),
			filepath.Join(track.StorageDir, "folder.jpg"),
		)
	}
	if metadataDir := filepath.Dir(strings.TrimSpace(track.MetadataPath)); metadataDir != "." && metadataDir != "" {
		candidates = append(candidates,
			filepath.Join(metadataDir, "cover.jpg"),
			filepath.Join(metadataDir, "cover.png"),
			filepath.Join(metadataDir, "folder.jpg"),
		)
	}
	for _, candidate := range uniquePaths(candidates) {
		if pathExists(candidate) {
			return candidate
		}
	}
	return ""
}

func resolveTrackVideoPath(track TrackRecord) string {
	candidates := []string{}
	if strings.TrimSpace(track.VideoPath) != "" {
		candidates = append(candidates, track.VideoPath)
	}
	audioBase := strings.TrimSuffix(baseNameOrEmpty(track.AudioPath), filepath.Ext(track.AudioPath))
	if strings.TrimSpace(track.StorageDir) != "" {
		if strings.TrimSpace(track.VideoPath) != "" {
			candidates = append(candidates, filepath.Join(track.StorageDir, filepath.Base(track.VideoPath)))
		}
		if audioBase != "" {
			candidates = append(candidates, filepath.Join(track.StorageDir, audioBase+".mp4"))
		}
		candidates = append(candidates, filepath.Join(track.StorageDir, "video.mp4"))
	}
	if metadataDir := filepath.Dir(strings.TrimSpace(track.MetadataPath)); metadataDir != "." && metadataDir != "" {
		if strings.TrimSpace(track.VideoPath) != "" {
			candidates = append(candidates, filepath.Join(metadataDir, filepath.Base(track.VideoPath)))
		}
		if audioBase != "" {
			candidates = append(candidates, filepath.Join(metadataDir, audioBase+".mp4"))
		}
		candidates = append(candidates, filepath.Join(metadataDir, "video.mp4"))
	}
	for _, candidate := range uniquePaths(candidates) {
		if pathExists(candidate) {
			return candidate
		}
	}
	return ""
}

func uniquePaths(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = filepath.Clean(strings.TrimSpace(value))
		if value == "." || value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func fileSize(info os.FileInfo) int64 {
	if info == nil {
		return 0
	}
	return info.Size()
}

func fileModTime(info os.FileInfo) time.Time {
	if info == nil {
		return time.Time{}
	}
	return info.ModTime().UTC()
}

func fileState(label, path string) TrackFileState {
	_, err := os.Stat(path)
	return TrackFileState{
		Label:  label,
		Path:   path,
		Exists: err == nil,
	}
}

func (a *App) RevealPath(path string) error {
	if path == "" {
		return errors.New("empty path")
	}
	validated, err := validatePathWithinRoots(path, []string{a.info.LibraryRoot, a.info.AppDataDir, a.info.IncomingDir, a.info.CacheDir}, true)
	if err != nil {
		return fmt.Errorf("reveal path: %w", err)
	}
	target := validated
	if info, statErr := os.Stat(validated); statErr == nil && !info.IsDir() {
		target = filepath.Dir(validated)
	}
	switch sysruntime.GOOS {
	case "darwin":
		args := []string{"open"}
		if _, statErr := os.Stat(validated); statErr == nil {
			args = []string{"open", "-R", validated}
		} else {
			args = []string{"open", target}
		}
		return exec.Command(args[0], args[1:]...).Start()
	case "windows":
		if _, statErr := os.Stat(validated); statErr == nil {
			return exec.Command("explorer", "/select,", validated).Start()
		}
		return exec.Command("explorer", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}

func (a *App) snapshotLocked() AppState {
	jobs := append([]Job{}, a.jobs...)
	sort.SliceStable(jobs, func(i, j int) bool {
		left := jobQueueSortRank(jobs[i].Status)
		right := jobQueueSortRank(jobs[j].Status)
		if left != right {
			return left < right
		}
		switch left {
		case 0:
			if !jobs[i].StartedAt.IsZero() && !jobs[j].StartedAt.IsZero() && !jobs[i].StartedAt.Equal(jobs[j].StartedAt) {
				return jobs[i].StartedAt.Before(jobs[j].StartedAt)
			}
		case 1:
			if !jobs[i].FinishedAt.IsZero() && !jobs[j].FinishedAt.IsZero() && !jobs[i].FinishedAt.Equal(jobs[j].FinishedAt) {
				return jobs[i].FinishedAt.After(jobs[j].FinishedAt)
			}
		case 2:
			if !jobs[i].CreatedAt.IsZero() && !jobs[j].CreatedAt.IsZero() && !jobs[i].CreatedAt.Equal(jobs[j].CreatedAt) {
				return jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
			}
		}
		if !jobs[i].CreatedAt.IsZero() && !jobs[j].CreatedAt.IsZero() && !jobs[i].CreatedAt.Equal(jobs[j].CreatedAt) {
			return jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
		}
		return jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
	})
	recentTracks := a.recentTracksLocked()
	playlists := append([]Playlist{}, a.playlists...)
	for i := range playlists {
		playlists[i].TrackIDs = append([]string{}, playlists[i].TrackIDs...)
	}
	importHistory := append([]ImportHistoryEntry{}, a.importHistory...)
	for i := range jobs {
		jobs[i].StageStatuses = jobStageStatusesFor(jobs[i])
	}
	return AppState{
		Settings:         publicSettings(a.settings),
		Stats:            a.statsLocked(),
		Health:           a.libraryHealthLocked(),
		Workers:          a.workerStatsLocked(jobs),
		GenreBuckets:     summarizeCatalogGenreCounts(a.catalog),
		YearBuckets:      summarizeCatalogYearCounts(a.catalog),
		BuildInfo:        a.buildInfo,
		UpdateInfo:       a.updateInfo,
		Diagnostics:      a.diagnostics,
		AIStatus:         a.aiStatus,
		Jobs:             jobs,
		ImportHistory:    importHistory,
		LibraryTracks:    a.libraryTracksLocked(),
		RecentTracks:     recentTracks,
		Playlists:        playlists,
		Playback:         a.player.snapshot(),
		PromptFiles:      promptInfos(a.promptLibrary),
		ToolStatus:       a.toolStatusLocked(),
		ToolReadiness:    a.toolReadinessLocked(),
		RootInfo:         a.info,
		WindowFullscreen: a.window != nil && a.window.IsFullscreen(),
	}
}

func (a *App) workerStatsLocked(jobs []Job) WorkerStats {
	stats := WorkerStats{
		Audio: WorkerClassStats{
			Capacity: a.maxConcurrentDownloadsLocked(),
		},
		Video: WorkerClassStats{
			Capacity: a.maxConcurrentVideoDownloadsLocked(),
		},
		Enrichment: WorkerClassStats{
			Capacity: a.maxConcurrentEnrichmentRequestsLocked(),
		},
		Lyrics: WorkerClassStats{
			Capacity: a.maxConcurrentLyricsRequestsLocked(),
		},
		Artwork: WorkerClassStats{
			Capacity:         a.maxConcurrentEnrichmentRequestsLocked(),
			SharedCapacityOf: string(workerWorkClassEnrichment),
		},
	}
	queued := map[workerWorkClass]int{}
	jobsByID := make(map[string]Job, len(jobs))
	for _, job := range jobs {
		jobsByID[job.ID] = job
	}
	// The queue slice is the admission source of truth. Jobs can remain
	// queued in persisted state while not being eligible for admission.
	for _, jobID := range a.jobQueue {
		job, ok := jobsByID[jobID]
		if !ok || job.Status != "queued" {
			continue
		}
		if job.DownloadVideo {
			queued[workerWorkClassVideo]++
		} else {
			queued[workerWorkClassAudio]++
		}
		if jobHasTrackWork(job) {
			queued[workerWorkClassEnrichment]++
			queued[workerWorkClassLyrics]++
			queued[workerWorkClassArtwork]++
		}
	}
	stats.Audio.Queued = queued[workerWorkClassAudio]
	stats.Video.Queued = queued[workerWorkClassVideo]
	stats.Enrichment.Queued = queued[workerWorkClassEnrichment]
	stats.Lyrics.Queued = queued[workerWorkClassLyrics]
	stats.Artwork.Queued = queued[workerWorkClassArtwork]

	a.workerMu.Lock()
	for _, state := range a.workerStates {
		if !state.busy {
			continue
		}
		stats.ActiveWorkers++
		switch workerWorkClass(state.workClass) {
		case workerWorkClassVideo:
			// Video.Active below reports the actual bounded video slot. The
			// legacy VideoActive field remains the running-job count.
		case workerWorkClassAudio:
			stats.Audio.Active++
		}
	}
	a.workerMu.Unlock()
	stats.MaxWorkers = a.maxConcurrentDownloadsLocked()
	videoActive, enrichmentActive, artworkActive, lyricsActive := a.activeThroughputCounts()
	stats.Video.Active = videoActive
	stats.Enrichment.Active = maxInt(enrichmentActive-artworkActive, 0)
	stats.Artwork.Active = artworkActive
	stats.Lyrics.Active = lyricsActive
	for _, job := range jobs {
		if job.DownloadVideo {
			switch job.Status {
			case "running":
				stats.VideoActive++
			case "queued":
				stats.VideoQueued++
			}
		}
	}
	stats.Throttle = workerThrottleStateLocked(a.downloadStats.throttle, time.Now().UTC())
	applyWorkerBlockedState(&stats.Audio, a.queueFrozen, stats.Throttle.Active, stats.Audio.Queued > 0, "")
	applyWorkerBlockedState(&stats.Video, a.queueFrozen, stats.Throttle.Active, stats.Video.Queued > 0, func() string {
		if stats.Video.Queued > 0 && stats.Video.Active >= stats.Video.Capacity {
			return workerBlockedReasonVideoCapacity
		}
		return ""
	}())
	applyWorkerBlockedState(&stats.Enrichment, a.queueFrozen, false, stats.Enrichment.Queued > 0, "")
	applyWorkerBlockedState(&stats.Lyrics, a.queueFrozen, false, stats.Lyrics.Queued > 0, "")
	applyWorkerBlockedState(&stats.Artwork, a.queueFrozen, false, stats.Artwork.Queued > 0, "")
	switch {
	case a.queueFrozen && hasQueuedWorkerWork(stats):
		stats.BlockedReason = workerBlockedReasonQueuePaused
	case stats.Video.Blocked && stats.Video.BlockedReason == workerBlockedReasonVideoCapacity:
		stats.BlockedReason = workerBlockedReasonVideoCapacity
	case stats.Throttle.Active && hasQueuedWorkerWork(stats):
		stats.BlockedReason = workerBlockedReasonThrottle
	}
	return stats
}

func jobHasTrackWork(job Job) bool {
	switch job.Kind {
	case "local-file", "url", "reprocess-track":
		return true
	default:
		return job.ParentJobID != ""
	}
}

func workerThrottleStateLocked(until, now time.Time) WorkerThrottleState {
	if until.IsZero() || !now.Before(until) {
		return WorkerThrottleState{}
	}
	return WorkerThrottleState{
		Active: true,
		Until:  until,
		Reason: workerThrottleReasonAuthCheck,
	}
}

func applyWorkerBlockedState(stats *WorkerClassStats, queueFrozen, throttled, hasQueued bool, specificReason string) {
	if !hasQueued {
		return
	}
	reason := specificReason
	switch {
	case queueFrozen:
		reason = workerBlockedReasonQueuePaused
	case reason == "" && throttled:
		reason = workerBlockedReasonThrottle
	}
	if reason == "" {
		return
	}
	stats.Blocked = true
	stats.BlockedReason = reason
}

func hasQueuedWorkerWork(stats WorkerStats) bool {
	return stats.Audio.Queued > 0 || stats.Video.Queued > 0 || stats.Enrichment.Queued > 0 || stats.Lyrics.Queued > 0 || stats.Artwork.Queued > 0
}

func jobQueueSortRank(status string) int {
	switch status {
	case "running":
		return 0
	case "completed", "failed", "stopped":
		return 1
	case "queued":
		return 2
	default:
		return 3
	}
}

func jobStageStatusesFor(job Job) JobStageStatuses {
	detailStages := parseJobStageStatusDetail(job.Detail)
	status := strings.ToLower(strings.TrimSpace(job.Status))
	progressStatus := func(progress int, runningValue, completeValue string) string {
		switch status {
		case "failed":
			return "Failed"
		case "stopped":
			return "Stopped"
		case "completed":
			if completeValue != "" {
				return completeValue
			}
			return "Completed"
		case "running":
			if progress >= 100 {
				if completeValue != "" {
					return completeValue
				}
				return "Completed"
			}
			if progress > 0 {
				if runningValue != "" {
					return runningValue
				}
				return "Running"
			}
			return "Queued"
		default:
			return "Queued"
		}
	}
	valueFor := func(keys ...string) string {
		for _, key := range keys {
			if value := normalizeStageStatusText(detailStages[strings.ToLower(strings.TrimSpace(key))]); value != "" {
				return value
			}
		}
		return ""
	}
	download := valueFor("download", "song")
	if download == "" {
		download = progressStatus(job.DownloadProgress, "Running", "Downloaded")
	}
	metadata := valueFor("ai metadata", "metadata")
	if metadata == "" {
		metadata = progressStatus(job.MetadataProgress, "Running", "Metadata discovered")
	}
	musicBrainz := valueFor("musicbrainz", "music brainz")
	if musicBrainz == "" {
		if status == "completed" {
			musicBrainz = "Matched"
		} else if status == "failed" || status == "stopped" {
			musicBrainz = normalizeStageStatusText(status)
		} else if job.MetadataProgress > 0 {
			musicBrainz = "Running"
		} else {
			musicBrainz = "Queued"
		}
	}
	lyrics := valueFor("lyrics")
	if lyrics == "" {
		lyrics = progressStatus(job.LyricsProgress, "Running", "Matched")
	}
	artwork := valueFor("album art", "artwork")
	if artwork == "" {
		if status == "completed" {
			artwork = "Downloaded"
		} else if status == "failed" || status == "stopped" {
			artwork = normalizeStageStatusText(status)
		} else if job.MetadataProgress >= 75 {
			artwork = "Running"
		} else {
			artwork = "Queued"
		}
	}
	video := valueFor("video")
	if video == "" {
		if status == "completed" {
			video = "Downloaded"
		} else if status == "failed" || status == "stopped" {
			video = normalizeStageStatusText(status)
		} else if status == "running" {
			video = "Running"
		} else {
			video = "Queued"
		}
	}
	finalize := valueFor("finalize", "finalise", "song finalization", "song")
	if finalize == "" {
		switch status {
		case "completed":
			finalize = "Completed"
		case "failed":
			finalize = "Failed"
		case "stopped":
			finalize = "Stopped"
		case "running":
			finalize = "Running"
		default:
			finalize = "Queued"
		}
	}
	return JobStageStatuses{
		Download:    normalizeStageStatusText(download),
		Metadata:    normalizeStageStatusText(metadata),
		MusicBrainz: normalizeStageStatusText(musicBrainz),
		Lyrics:      normalizeStageStatusText(lyrics),
		Artwork:     normalizeStageStatusText(artwork),
		Video:       normalizeStageStatusText(video),
		Finalize:    normalizeStageStatusText(finalize),
	}
}

func parseJobStageStatusDetail(detail string) map[string]string {
	stages := map[string]string{}
	for _, chunk := range strings.FieldsFunc(detail, func(r rune) bool { return r == ';' || r == '\n' }) {
		chunk = strings.TrimSpace(strings.Trim(chunk, " ·"))
		if chunk == "" {
			continue
		}
		label, value, ok := strings.Cut(chunk, ":")
		if !ok {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(label))
		if key == "" {
			continue
		}
		stages[key] = strings.TrimSpace(value)
	}
	return stages
}

func normalizeStageStatusText(value string) string {
	value = strings.TrimSpace(strings.TrimSuffix(value, "."))
	if value == "" {
		return ""
	}
	value = strings.ToLower(value)
	return strings.ToUpper(value[:1]) + value[1:]
}

func publicSettings(settings storedSettings) PublicSettings {
	return PublicSettings{
		LibraryRoot:                     settings.LibraryRoot,
		AIBaseURL:                       settings.AIBaseURL,
		AIModel:                         settings.AIModel,
		Provider:                        settings.Provider,
		APIKeyConfigured:                settings.APIKey != "",
		UpdateManifestURL:               settings.UpdateManifestURL,
		YTDLPPath:                       settings.YTDLPPath,
		FFmpegPath:                      settings.FFmpegPath,
		YTDLPCookiesPath:                settings.YTDLPCookiesPath,
		YTDLPCookiesFromBrowser:         settings.YTDLPCookiesFromBrowser,
		VideoDownloadMode:               normalizeVideoDownloadMode(settings.VideoDownloadMode),
		DownloadMusicVideo:              normalizeVideoDownloadMode(settings.VideoDownloadMode) == videoDownloadModeDuringImport,
		KeepOriginalAudio:               settings.KeepOriginalAudio,
		MaxConcurrentDownloads:          settings.MaxConcurrentDownloads,
		MaxConcurrentVideoDownloads:     settings.MaxConcurrentVideoDownloads,
		MaxConcurrentEnrichmentRequests: settings.MaxConcurrentEnrichmentRequests,
		MaxConcurrentLyricsRequests:     settings.MaxConcurrentLyricsRequests,
		ThrottleOnYTDLPBotErrors:        settings.ThrottleOnYTDLPBotErrors,
	}
}

func promptInfos(prompts *PromptLibrary) []PromptInfo {
	if prompts == nil {
		return []PromptInfo{}
	}
	return prompts.Info()
}

func (a *App) statsLocked() LibraryStats {
	stats := LibraryStats{JobCount: len(a.jobs)}
	for _, job := range a.jobs {
		switch job.Status {
		case "queued", "running":
			stats.PendingJobs++
		case "failed":
			stats.FailedJobs++
		}
	}
	artistSet := map[string]struct{}{}
	albumSet := map[string]struct{}{}
	for _, track := range a.catalog.Tracks {
		stats.TrackCount++
		artistSet[strings.ToLower(track.Artist)] = struct{}{}
		albumSet[strings.ToLower(track.Artist+"::"+track.Album)] = struct{}{}
	}
	stats.ArtistCount = len(artistSet)
	stats.AlbumCount = len(albumSet)
	return stats
}

func (a *App) libraryHealthLocked() LibraryHealth {
	health := LibraryHealth{}
	for _, track := range a.catalog.Tracks {
		health.TotalTracks++
		if isTrackUnprocessed(track) {
			health.UnprocessedTracks++
		}
		if track.AudioPath == "" || !pathExists(track.AudioPath) {
			health.MissingAudio++
		}
		if track.MetadataPath == "" || !pathExists(track.MetadataPath) {
			health.MissingMetadata++
		}
		if track.LyricsPath == "" || !pathExists(track.LyricsPath) {
			health.MissingLyrics++
		}
		if track.LRCPath == "" || !pathExists(track.LRCPath) {
			health.MissingTimed++
		}
		if track.AudioPath != "" && pathExists(track.AudioPath) && track.MetadataPath != "" && pathExists(track.MetadataPath) {
			health.ReadyTracks++
		}
	}
	return health
}

func isTrackUnprocessed(track TrackRecord) bool {
	switch strings.ToLower(strings.TrimSpace(track.MetadataConfidence)) {
	case "high":
		return false
	case "medium":
		return false
	default:
		return true
	}
}

func (a *App) toolStatusLocked() ToolStatus {
	return ToolStatus{
		YTDLP:  a.toolPath(a.settings.YTDLPPath, "yt-dlp") != "",
		FFmpeg: a.toolPath(a.settings.FFmpegPath, "ffmpeg") != "",
		AI:     a.ai != nil && a.ai.available(),
	}
}

func (a *App) toolPath(configured, name string) string {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if resolved, err := validateExecutablePath(configured); err == nil {
			return resolved
		}
		if resolved, err := exec.LookPath(configured); err == nil {
			return resolved
		}
		return ""
	}
	if resolved, err := exec.LookPath(name); err == nil {
		return resolved
	}
	return ""
}

func (a *App) recentTracksLocked() []TrackRecord {
	tracks := append([]TrackRecord{}, a.catalog.Tracks...)
	for i := range tracks {
		tracks[i] = a.trackWithArtworkMediaURL(tracks[i])
	}
	sort.SliceStable(tracks, func(i, j int) bool {
		return tracks[i].CreatedAt.After(tracks[j].CreatedAt)
	})
	if len(tracks) > 8 {
		tracks = tracks[:8]
	}
	return tracks
}

func (a *App) libraryTracksLocked() []TrackRecord {
	tracks := append([]TrackRecord{}, a.catalog.Tracks...)
	for i := range tracks {
		tracks[i] = a.trackWithArtworkMediaURL(tracks[i])
	}
	sort.SliceStable(tracks, func(i, j int) bool {
		return tracks[i].CreatedAt.After(tracks[j].CreatedAt)
	})
	return tracks
}

func (a *App) jobByIDLocked(jobID string) (Job, bool) {
	for _, job := range a.jobs {
		if job.ID == jobID {
			return job, true
		}
	}
	return Job{}, false
}

func (a *App) addJobLocked(kind, input, sourceRoot string) Job {
	job := Job{
		ID:            shortID(),
		Kind:          kind,
		Input:         input,
		SourceRoot:    sourceRoot,
		DownloadVideo: kind == "url" && normalizeVideoDownloadMode(a.settings.VideoDownloadMode) == videoDownloadModeDuringImport,
		Status:        "queued",
		Detail:        "Waiting for worker",
		CreatedAt:     time.Now().UTC(),
	}
	a.jobs = append(a.jobs, job)
	a.catalog.Jobs = append(a.catalog.Jobs, job)
	a.enqueueJobLocked(job.ID)
	a.requestWorkerPoolReconcile()
	logEvent("job_queued", "job_id", job.ID, "kind", kind, "source_root", sourceRoot, "input", input)
	a.logQueueStateLocked("queue_state_changed", "reason", "job_queued", "job_id", job.ID)
	return job
}

func (a *App) addPlaylistJobLocked(job Job) Job {
	job.ID = shortID()
	job.Status = "queued"
	if strings.TrimSpace(job.Detail) == "" {
		job.Detail = "Waiting for worker"
	}
	job.CreatedAt = time.Now().UTC()
	a.jobs = append(a.jobs, job)
	a.catalog.Jobs = append(a.catalog.Jobs, job)
	a.enqueueJobLocked(job.ID)
	logEvent("job_queued", "job_id", job.ID, "kind", job.Kind, "parent_job_id", job.ParentJobID, "playlist_index", job.PlaylistIndex)
	a.logQueueStateLocked("queue_state_changed", "reason", "playlist_job_queued", "job_id", job.ID)
	return job
}

func (a *App) StartJob(jobID string) (AppState, error) {
	a.mu.Lock()
	var shouldResume bool
	for i := range a.jobs {
		if a.jobs[i].ID != jobID {
			continue
		}
		logEvent("job_start_requested", "job_id", jobID, "status", a.jobs[i].Status, "kind", a.jobs[i].Kind, "parent_job_id", a.jobs[i].ParentJobID)
		switch a.jobs[i].Status {
		case "running":
			log.Printf("job start requested but already running: %s", jobID)
			state := a.snapshotLocked()
			a.mu.Unlock()
			return state, nil
		case "completed":
			log.Printf("job start requested but already completed: %s", jobID)
			state := a.snapshotLocked()
			a.mu.Unlock()
			return state, nil
		case "queued":
			if a.queueFrozen {
				log.Printf("resuming queue for job start: %s", jobID)
				a.queueFrozen = false
				shouldResume = true
			}
			a.jobs[i].Detail = "Waiting for worker"
		default:
			log.Printf("starting job: %s (%s)", jobID, a.jobs[i].Status)
			if a.jobs[i].Kind == "playlist" && a.jobs[i].ParentJobID == "" {
				a.jobs[i].Status = "running"
				a.jobs[i].Detail = "This is a playlist. Processing time extended."
				a.jobs[i].Error = ""
				a.jobs[i].StartedAt = time.Now().UTC()
				a.jobs[i].FinishedAt = time.Time{}
				a.queueFrozen = false
				a.resumePlaylistChildrenLocked(jobID)
				shouldResume = true
			} else {
				a.jobs[i].Status = "queued"
				a.jobs[i].Detail = "Waiting for worker"
				a.jobs[i].Error = ""
				a.jobs[i].StartedAt = time.Time{}
				a.jobs[i].FinishedAt = time.Time{}
				a.jobs[i].DownloadProgress = 0
				a.jobs[i].MetadataProgress = 0
				a.jobs[i].LyricsProgress = 0
				a.jobs[i].ResultTitle = ""
				a.jobs[i].ResultArtist = ""
				a.jobs[i].ResultAlbum = ""
				a.queueFrozen = false
				a.enqueueJobLocked(jobID)
				shouldResume = true
			}
		}
		a.syncCatalogJobLocked(a.jobs[i])
		a.persistLocked()
		a.logQueueStateLocked("queue_state_changed", "reason", "job_started", "job_id", jobID)
		state := a.snapshotLocked()
		a.mu.Unlock()
		if shouldResume {
			a.requestWorkerPoolReconcile()
		}
		a.emitStateLocked(state)
		return state, nil
	}
	a.mu.Unlock()
	return AppState{}, fmt.Errorf("job %s not found", jobID)
}

func (a *App) StopJob(jobID string) (AppState, error) {
	a.mu.Lock()
	var cancel context.CancelFunc
	for i := range a.jobs {
		if a.jobs[i].ID != jobID {
			continue
		}
		parentID := a.jobs[i].ParentJobID
		logEvent("job_stop_requested", "job_id", jobID, "status", a.jobs[i].Status, "kind", a.jobs[i].Kind, "parent_job_id", parentID)
		if a.jobs[i].Status != "running" && a.jobs[i].Status != "queued" {
			log.Printf("job stop requested but not active: %s (%s)", jobID, a.jobs[i].Status)
			state := a.snapshotLocked()
			a.mu.Unlock()
			return state, nil
		}
		log.Printf("stopping job: %s (%s)", jobID, a.jobs[i].Status)
		wasQueued := a.jobs[i].Status == "queued"
		if a.jobs[i].Kind == "playlist" && a.jobs[i].ParentJobID == "" {
			a.stopPlaylistChildrenLocked(jobID)
			a.jobs[i].Status = "stopped"
			a.jobs[i].Detail = "Stopped by user"
			a.jobs[i].Error = ""
			a.jobs[i].FinishedAt = time.Now().UTC()
		} else {
			a.jobs[i].Status = "stopped"
			a.jobs[i].Detail = "Stopped by user"
			a.jobs[i].Error = ""
			a.jobs[i].FinishedAt = time.Now().UTC()
			if wasQueued {
				a.removeQueuedJobLocked(jobID)
			}
		}
		a.syncCatalogJobLocked(a.jobs[i])
		if parentID != "" {
			a.refreshPlaylistBatchLocked(parentID)
		}
		cancel = a.jobCancels[jobID]
		delete(a.jobCancels, jobID)
		a.persistLocked()
		a.logQueueStateLocked("queue_state_changed", "reason", "job_stopped", "job_id", jobID)
		state := a.snapshotLocked()
		a.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		a.requestWorkerPoolReconcile()
		a.emitStateLocked(state)
		return state, nil
	}
	a.mu.Unlock()
	return AppState{}, fmt.Errorf("job %s not found", jobID)
}

func (a *App) PauseAllJobs() (AppState, error) {
	a.mu.Lock()
	logEvent("queue_pause_all_requested")
	wasFrozen := a.queueFrozen
	a.queueFrozen = true
	var cancels []context.CancelFunc
	now := time.Now().UTC()
	changed := !wasFrozen
	for i := range a.jobs {
		if a.jobs[i].Status != "running" {
			continue
		}
		log.Printf("pausing job: %s (%s)", a.jobs[i].ID, a.jobs[i].Status)
		if a.jobs[i].Kind == "playlist" && a.jobs[i].ParentJobID == "" {
			a.pausePlaylistChildrenLocked(a.jobs[i].ID)
		}
		if cancel := a.jobCancels[a.jobs[i].ID]; cancel != nil {
			cancels = append(cancels, cancel)
			delete(a.jobCancels, a.jobs[i].ID)
		}
		a.jobs[i].Status = "stopped"
		a.jobs[i].Detail = "Stopped by user"
		a.jobs[i].Error = ""
		a.jobs[i].FinishedAt = now
		a.syncCatalogJobLocked(a.jobs[i])
		changed = true
	}
	if changed {
		a.persistLocked()
		a.logQueueStateLocked("queue_state_changed", "reason", "queue_pause_all", "changed", changed)
		logEvent("queue_pause_all_applied")
	}
	state := a.snapshotLocked()
	a.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	if changed {
		a.emitStateLocked(state)
		a.requestWorkerPoolReconcile()
	}
	return state, nil
}

func (a *App) DeleteAllJobs() (AppState, error) {
	logEvent("queue_delete_all_requested")
	return a.deleteJobsByPredicate(func(Job) bool { return true }, "deleting all jobs")
}

func (a *App) DeleteDoneJobs() (AppState, error) {
	logEvent("queue_delete_done_requested")
	return a.deleteJobsByPredicate(func(job Job) bool {
		return job.Status != "queued" && job.Status != "running"
	}, "deleting done jobs")
}

func (a *App) DeleteQueuedJobs() (AppState, error) {
	logEvent("queue_delete_queued_requested")
	return a.deleteJobsByPredicate(func(job Job) bool {
		return job.Status == "queued"
	}, "deleting queued jobs")
}

func (a *App) deleteJobsByPredicate(match func(Job) bool, logMessage string) (AppState, error) {
	a.mu.Lock()
	wasFrozen := a.queueFrozen
	a.queueFrozen = true
	var cancels []context.CancelFunc
	filtered := make([]Job, 0, len(a.jobs))
	removedIDs := make(map[string]struct{})
	affectedRoots := make(map[string]struct{})
	changed := false
	for _, job := range a.jobs {
		if _, removed := removedIDs[job.ID]; removed {
			continue
		}
		if !match(job) {
			filtered = append(filtered, job)
			continue
		}
		log.Printf("%s: %s (%s)", logMessage, job.ID, job.Status)
		logEvent("queue_delete_match", "operation", logMessage, "job_id", job.ID, "status", job.Status, "kind", job.Kind, "parent_job_id", job.ParentJobID)
		if job.Kind == "playlist" && job.ParentJobID == "" {
			for _, child := range a.playlistChildrenLocked(job.ID) {
				removedIDs[child.ID] = struct{}{}
			}
		}
		if job.ParentJobID != "" {
			affectedRoots[job.ParentJobID] = struct{}{}
		}
		removedIDs[job.ID] = struct{}{}
		if cancel := a.jobCancels[job.ID]; cancel != nil {
			cancels = append(cancels, cancel)
			delete(a.jobCancels, job.ID)
		}
		changed = true
	}
	for removedID := range removedIDs {
		if cancel := a.jobCancels[removedID]; cancel != nil {
			cancels = append(cancels, cancel)
			delete(a.jobCancels, removedID)
		}
	}
	if changed {
		a.jobs = filtered
		a.catalog.Jobs = append([]Job(nil), filtered...)
		if len(removedIDs) > 0 && len(a.jobQueue) > 0 {
			filteredQueue := a.jobQueue[:0]
			for _, jobID := range a.jobQueue {
				if _, removed := removedIDs[jobID]; removed {
					continue
				}
				filteredQueue = append(filteredQueue, jobID)
			}
			a.jobQueue = append([]string(nil), filteredQueue...)
		}
		for rootID := range affectedRoots {
			a.refreshPlaylistBatchLocked(rootID)
		}
		a.persistLocked()
		a.logQueueStateLocked("queue_state_changed", "reason", logMessage, "removed_count", len(removedIDs))
		logEvent("queue_delete_applied", "operation", logMessage, "removed_count", len(removedIDs))
	}
	state := a.snapshotLocked()
	a.queueFrozen = wasFrozen
	a.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	if changed {
		a.emitStateLocked(state)
		a.requestWorkerPoolReconcile()
	}
	return state, nil
}

func (a *App) DeleteJob(jobID string) (AppState, error) {
	a.mu.Lock()
	var cancel context.CancelFunc
	var targetParent string
	var playlistRoot bool
	var found bool
	for i := range a.jobs {
		if a.jobs[i].ID != jobID {
			continue
		}
		log.Printf("deleting job: %s (%s)", jobID, a.jobs[i].Status)
		logEvent("job_delete_requested", "job_id", jobID, "status", a.jobs[i].Status, "kind", a.jobs[i].Kind, "parent_job_id", a.jobs[i].ParentJobID)
		targetParent = a.jobs[i].ParentJobID
		playlistRoot = a.jobs[i].Kind == "playlist" && a.jobs[i].ParentJobID == ""
		cancel = a.jobCancels[jobID]
		delete(a.jobCancels, jobID)
		found = true
		break
	}
	if !found {
		a.mu.Unlock()
		return AppState{}, fmt.Errorf("job %s not found", jobID)
	}
	if playlistRoot {
		a.deletePlaylistChildrenLocked(jobID)
	}
	for i := range a.jobs {
		if a.jobs[i].ID != jobID {
			continue
		}
		if a.jobs[i].Status == "queued" {
			a.removeQueuedJobLocked(jobID)
		}
		a.jobs = append(a.jobs[:i], a.jobs[i+1:]...)
		break
	}
	for j := range a.catalog.Jobs {
		if a.catalog.Jobs[j].ID == jobID {
			a.catalog.Jobs = append(a.catalog.Jobs[:j], a.catalog.Jobs[j+1:]...)
			break
		}
	}
	if targetParent != "" {
		a.refreshPlaylistBatchLocked(targetParent)
	}
	a.persistLocked()
	a.logQueueStateLocked("queue_state_changed", "reason", "job_deleted", "job_id", jobID)
	state := a.snapshotLocked()
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	a.requestWorkerPoolReconcile()
	a.emitStateLocked(state)
	logEvent("job_delete_applied", "job_id", jobID, "playlist_root", playlistRoot)
	return state, nil
}

func (a *App) addTrack(track TrackRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.catalog.Tracks = append([]TrackRecord{track}, a.catalog.Tracks...)
	a.rebuildTrackIndexLocked()
	a.persistLocked()
	a.emitStateLocked(a.snapshotLocked())
}

func (a *App) replaceTrack(track TrackRecord) (TrackRecord, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.catalog.Tracks {
		if a.catalog.Tracks[i].ID == track.ID {
			old := a.catalog.Tracks[i]
			a.catalog.Tracks[i] = track
			a.rebuildTrackIndexLocked()
			a.persistLocked()
			a.emitStateLocked(a.snapshotLocked())
			return old, true
		}
	}
	a.catalog.Tracks = append([]TrackRecord{track}, a.catalog.Tracks...)
	a.rebuildTrackIndexLocked()
	a.persistLocked()
	a.emitStateLocked(a.snapshotLocked())
	return TrackRecord{}, false
}

func (a *App) addReprocessJobLocked(job Job) Job {
	job.ID = shortID()
	job.Status = "queued"
	if strings.TrimSpace(job.Detail) == "" {
		job.Detail = "Waiting for worker"
	}
	job.CreatedAt = time.Now().UTC()
	a.jobs = append(a.jobs, job)
	a.catalog.Jobs = append(a.catalog.Jobs, job)
	a.enqueueJobLocked(job.ID)
	a.requestWorkerPoolReconcile()
	logEvent("job_queued", "job_id", job.ID, "kind", job.Kind, "track_id", job.TrackID, "input", job.Input)
	a.logQueueStateLocked("queue_state_changed", "reason", "reprocess_job_queued", "job_id", job.ID)
	return job
}

func (a *App) trackForReprocess(trackID string) (TrackRecord, string, songstore.SongMetadata, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	track, ok := a.trackIndex[trackID]
	if !ok {
		for _, candidate := range a.catalog.Tracks {
			if candidate.ID == trackID {
				track = candidate
				ok = true
				break
			}
		}
	}
	if !ok {
		return TrackRecord{}, "", songstore.SongMetadata{}, fmt.Errorf("track %s not found", trackID)
	}
	metadataPath := strings.TrimSpace(track.MetadataPath)
	if metadataPath == "" {
		metadataPath = strings.TrimSpace(track.BundlePath)
	}
	if metadataPath == "" {
		return TrackRecord{}, "", songstore.SongMetadata{}, fmt.Errorf("metadata path is not available for track %s", trackID)
	}
	metadata, err := songstore.LoadSongMetadata(metadataPath)
	if err != nil {
		metadata = metadataFromTrackRecord(track, metadataPath)
	} else {
		metadata = repairLoadedMetadata(metadataPath, metadata)
	}
	metadata = ensureMetadataSidecarPaths(track, metadata, metadataPath)
	return track, metadataPath, metadata, nil
}

func (a *App) cleanupReprocessedTrackFiles(oldTrack, newTrack TrackRecord) {
	oldPaths := []string{
		strings.TrimSpace(oldTrack.MetadataPath),
		strings.TrimSpace(oldTrack.AudioPath),
		strings.TrimSpace(oldTrack.LyricsPath),
		strings.TrimSpace(oldTrack.LRCPath),
		strings.TrimSpace(oldTrack.VideoPath),
	}
	newPaths := map[string]struct{}{
		filepath.Clean(strings.TrimSpace(newTrack.MetadataPath)): {},
		filepath.Clean(strings.TrimSpace(newTrack.AudioPath)):    {},
		filepath.Clean(strings.TrimSpace(newTrack.LyricsPath)):   {},
		filepath.Clean(strings.TrimSpace(newTrack.LRCPath)):      {},
		filepath.Clean(strings.TrimSpace(newTrack.VideoPath)):    {},
	}
	removed := 0
	for _, path := range oldPaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, keep := newPaths[filepath.Clean(path)]; keep {
			continue
		}
		if err := os.Remove(path); err == nil {
			removed++
			log.Printf("reprocess cleanup removed stale file: %s", path)
		}
	}
	if removed > 0 {
		logEvent("track_reprocess_cleanup_completed", "track_id", newTrack.ID, "removed_count", removed)
	}
}

func (a *App) localImportAlreadyExists(sourcePath, hash string) (TrackRecord, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cleanSource := filepath.Clean(sourcePath)
	for _, track := range a.catalog.Tracks {
		if track.SourceKind == "local-file" && track.SourceRef != "" && filepath.Clean(track.SourceRef) == cleanSource {
			return track, true
		}
		if hash != "" && track.Hash != "" && track.Hash == hash {
			return track, true
		}
	}
	return TrackRecord{}, false
}

func (a *App) shouldProcessJob(jobID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, job := range a.jobs {
		if job.ID == jobID {
			return job.Status == "queued"
		}
	}
	return false
}

func (a *App) GetPlaylists() []Playlist {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Playlist(nil), a.playlists...)
}

func (a *App) PlayTrack(trackID string, queueTrackIDs []string, queueSource string) (PlaybackState, error) {
	logEvent("playback_play_requested", "track_id", trackID, "queue_source", queueSource, "queue_length", len(queueTrackIDs))
	if err := a.player.play(trackID, queueTrackIDs, queueSource); err != nil {
		logEvent("playback_play_failed", "track_id", trackID, "queue_source", queueSource, "error", err.Error())
		return PlaybackState{}, err
	}
	state := a.player.snapshot()
	a.emitPlaybackState(state)
	return state, nil
}

func (a *App) TogglePlayback() (PlaybackState, error) {
	logEvent("playback_toggle_requested")
	if err := a.player.togglePlayback(); err != nil {
		logEvent("playback_toggle_failed", "error", err.Error())
		return PlaybackState{}, err
	}
	state := a.player.snapshot()
	a.emitPlaybackState(state)
	return state, nil
}

func (a *App) PausePlayback() (PlaybackState, error) {
	logEvent("playback_pause_requested")
	if err := a.player.pause(); err != nil {
		logEvent("playback_pause_failed", "error", err.Error())
		return PlaybackState{}, err
	}
	state := a.player.snapshot()
	a.emitPlaybackState(state)
	return state, nil
}

func (a *App) PlayNext() (PlaybackState, error) {
	logEvent("playback_next_requested")
	if err := a.player.next(); err != nil {
		logEvent("playback_next_failed", "error", err.Error())
		return PlaybackState{}, err
	}
	state := a.player.snapshot()
	a.emitPlaybackState(state)
	return state, nil
}

func (a *App) PlayPrevious() (PlaybackState, error) {
	logEvent("playback_previous_requested")
	if err := a.player.previous(); err != nil {
		logEvent("playback_previous_failed", "error", err.Error())
		return PlaybackState{}, err
	}
	state := a.player.snapshot()
	a.emitPlaybackState(state)
	return state, nil
}

func (a *App) SeekPlayback(ratio float64) (PlaybackState, error) {
	logEvent("playback_seek_requested", "ratio", ratio)
	if err := a.player.seek(ratio); err != nil {
		logEvent("playback_seek_failed", "ratio", ratio, "error", err.Error())
		return PlaybackState{}, err
	}
	state := a.player.snapshot()
	a.emitPlaybackState(state)
	return state, nil
}

func (a *App) SetPlaybackVolume(value float64) (PlaybackState, error) {
	logEvent("playback_volume_set", "value", value)
	a.player.setVolume(value)
	state := a.player.snapshot()
	a.emitPlaybackState(state)
	return state, nil
}

func (a *App) ToggleMute() (PlaybackState, error) {
	logEvent("playback_mute_toggled")
	a.player.toggleMute()
	state := a.player.snapshot()
	a.emitPlaybackState(state)
	return state, nil
}

func (a *App) ToggleShuffle() (PlaybackState, error) {
	logEvent("playback_shuffle_toggled")
	a.player.setShuffle(!a.player.snapshot().ShuffleEnabled)
	state := a.player.snapshot()
	a.emitPlaybackState(state)
	return state, nil
}

func (a *App) SetShuffleEnabled(enabled bool) (PlaybackState, error) {
	logEvent("playback_shuffle_set", "enabled", enabled)
	a.player.setShuffle(enabled)
	state := a.player.snapshot()
	a.emitPlaybackState(state)
	return state, nil
}

func (a *App) ToggleRepeatMode() (PlaybackState, error) {
	logEvent("playback_repeat_cycled")
	a.player.cycleRepeat()
	state := a.player.snapshot()
	a.emitPlaybackState(state)
	return state, nil
}

func (a *App) DownloadTrackVideo(trackID string) (AppState, error) {
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		logEvent("video_download_failed", "reason", "missing_track_id")
		return AppState{}, errors.New("track id is required")
	}
	a.mu.Lock()
	track, ok := a.trackIndex[trackID]
	if !ok {
		for _, candidate := range a.catalog.Tracks {
			if candidate.ID == trackID {
				track = candidate
				ok = true
				break
			}
		}
	}
	videoMode := normalizeVideoDownloadMode(a.settings.VideoDownloadMode)
	a.mu.Unlock()
	if videoMode == videoDownloadModeOff {
		logEvent("video_download_failed", "track_id", trackID, "reason", "video_downloads_disabled")
		return AppState{}, errors.New("video downloads are disabled in Settings > Downloader")
	}
	if !ok {
		logEvent("video_download_failed", "track_id", trackID, "reason", "track_not_found")
		return AppState{}, fmt.Errorf("track %s not found", trackID)
	}
	sourceURL := firstNonEmpty(track.SourceRef, track.VideoURL)
	if sourceURL == "" {
		logEvent("video_download_failed", "track_id", trackID, "reason", "missing_source_url")
		return AppState{}, errors.New("no video source url available")
	}
	metadataPath := strings.TrimSpace(track.MetadataPath)
	if metadataPath == "" {
		metadataPath = strings.TrimSpace(track.BundlePath)
	}
	if metadataPath == "" {
		logEvent("video_download_failed", "track_id", trackID, "reason", "missing_metadata_path")
		return AppState{}, errors.New("metadata path not available")
	}
	stageDir := filepath.Join(a.info.IncomingDir, "video-"+track.ID)
	logEvent("video_download_requested", "track_id", trackID, "source_url", sourceURL, "metadata_path", metadataPath)
	var videoStage string
	if err := a.withVideoDownloadSlot(context.Background(), func() error {
		var downloadErr error
		videoCtx, cancelVideo := context.WithTimeout(context.Background(), maxExternalImportDuration)
		defer cancelVideo()
		videoStage, downloadErr = a.downloadVideoFile(videoCtx, sourceURL, stageDir)
		return downloadErr
	}); err != nil {
		logEvent("video_download_failed", "track_id", trackID, "source_url", sourceURL, "error", err.Error())
		return AppState{}, err
	}
	metadata, err := songstore.LoadSongMetadata(metadataPath)
	if err != nil {
		return AppState{}, err
	}
	metadata = repairLoadedMetadata(metadataPath, metadata)
	finalVideoPath := track.VideoPath
	videoExt := filepath.Ext(videoStage)
	if videoExt == "" {
		videoExt = filepath.Ext(finalVideoPath)
	}
	if videoExt == "" {
		videoExt = ".mp4"
	}
	base := strings.TrimSuffix(filepath.Base(metadataPath), ".metadata.json")
	if strings.TrimSpace(finalVideoPath) == "" {
		finalVideoPath = filepath.Join(filepath.Dir(metadataPath), base+videoExt)
	} else if filepath.Ext(finalVideoPath) != videoExt {
		finalVideoPath = filepath.Join(filepath.Dir(finalVideoPath), strings.TrimSuffix(filepath.Base(finalVideoPath), filepath.Ext(finalVideoPath))+videoExt)
	}
	metadata.Video = songstore.SongVideo{
		Filename:  filepath.Base(finalVideoPath),
		Path:      finalVideoPath,
		Source:    "yt-dlp",
		URL:       sourceURL,
		FetchedAt: time.Now().UTC(),
	}
	if err := copyFile(videoStage, metadata.Video.Path); err != nil {
		logEvent("video_copy_failed", "track_id", trackID, "video_path", metadata.Video.Path, "error", err.Error())
		return AppState{}, err
	}
	if err := songstore.WriteMetadataJSON(metadataPath, metadata); err != nil {
		logEvent("video_metadata_write_failed", "track_id", trackID, "metadata_path", metadataPath, "error", err.Error())
		return AppState{}, err
	}
	a.rememberDownloadForURL(sourceURL, ytDLPInfo{
		ID:           metadata.Source.VideoID,
		Title:        metadata.Title,
		FullTitle:    metadata.Title,
		Channel:      firstNonEmpty(metadata.Source.Channel, track.SourceChannel),
		Uploader:     firstNonEmpty(metadata.Source.Channel, track.SourceChannel),
		WebpageURL:   sourceURL,
		ExtractorKey: firstNonEmpty(metadata.Source.Provider, track.SourceKind, "youtube"),
	}, track.AudioPath, metadata.Video.Path)
	if err := os.RemoveAll(stageDir); err != nil {
		log.Printf("failed to clean video stage for track %s: %v", trackID, err)
	}
	logEvent("video_download_completed", "track_id", trackID, "video_path", metadata.Video.Path, "metadata_path", metadataPath)
	return a.RescanLibrary()
}

func (a *App) ClearPlaybackQueue() (PlaybackState, error) {
	a.player.clearQueue()
	state := a.player.snapshot()
	a.emitPlaybackState(state)
	a.emitState()
	return state, nil
}

func (a *App) AddTrackToPlaybackQueue(trackID string) (PlaybackState, error) {
	a.player.addToQueue(trackID)
	state := a.player.snapshot()
	a.emitPlaybackState(state)
	a.emitState()
	return state, nil
}

func (a *App) RemoveTrackFromPlaybackQueue(trackID string) (PlaybackState, error) {
	a.player.removeFromQueue(trackID)
	state := a.player.snapshot()
	a.emitPlaybackState(state)
	a.emitState()
	return state, nil
}

func (a *App) ShufflePlaybackQueue() (PlaybackState, error) {
	a.player.shuffleQueue()
	state := a.player.snapshot()
	a.emitPlaybackState(state)
	a.emitState()
	return state, nil
}

func (a *App) MoveTrackInPlaybackQueue(trackID string, delta int) (PlaybackState, error) {
	a.player.moveQueue(trackID, delta)
	state := a.player.snapshot()
	a.emitPlaybackState(state)
	a.emitState()
	return state, nil
}

func (a *App) CreatePlaylist(name, description string) (Playlist, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		logEvent("playlist_create_failed", "reason", "missing_name")
		return Playlist{}, errors.New("playlist name is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now().UTC()
	playlist := Playlist{
		ID:          shortID(),
		Name:        name,
		Description: strings.TrimSpace(description),
		CreatedAt:   now,
		UpdatedAt:   now,
		TrackIDs:    []string{},
	}
	a.playlists = append([]Playlist{playlist}, a.playlists...)
	a.persistLocked()
	a.emitStateLocked(a.snapshotLocked())
	logEvent("playlist_created", "playlist_id", playlist.ID, "name", playlist.Name, "track_count", len(playlist.TrackIDs))
	return playlist, nil
}

func (a *App) RenamePlaylist(playlistID, name, description string) (Playlist, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		logEvent("playlist_rename_failed", "playlist_id", playlistID, "reason", "missing_name")
		return Playlist{}, errors.New("playlist name is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.playlists {
		if a.playlists[i].ID == playlistID {
			a.playlists[i].Name = name
			a.playlists[i].Description = strings.TrimSpace(description)
			a.playlists[i].UpdatedAt = time.Now().UTC()
			playlist := a.playlists[i]
			a.persistLocked()
			a.emitStateLocked(a.snapshotLocked())
			logEvent("playlist_renamed", "playlist_id", playlistID, "name", playlist.Name, "description", playlist.Description)
			return playlist, nil
		}
	}
	logEvent("playlist_rename_failed", "playlist_id", playlistID, "reason", "playlist_not_found")
	return Playlist{}, errors.New("playlist not found")
}

func (a *App) DeletePlaylist(playlistID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.playlists {
		if a.playlists[i].ID == playlistID {
			a.playlists = append(a.playlists[:i], a.playlists[i+1:]...)
			a.persistLocked()
			a.emitStateLocked(a.snapshotLocked())
			logEvent("playlist_deleted", "playlist_id", playlistID)
			return nil
		}
	}
	logEvent("playlist_delete_failed", "playlist_id", playlistID, "reason", "playlist_not_found")
	return errors.New("playlist not found")
}

func (a *App) AddTrackToPlaylist(playlistID, trackID string) (Playlist, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	pl, idx, ok := a.playlistByIDLocked(playlistID)
	if !ok {
		logEvent("playlist_add_track_failed", "playlist_id", playlistID, "track_id", trackID, "reason", "playlist_not_found")
		return Playlist{}, errors.New("playlist not found")
	}
	if _, ok := a.trackIndex[trackID]; !ok {
		logEvent("playlist_add_track_failed", "playlist_id", playlistID, "track_id", trackID, "reason", "track_not_found")
		return Playlist{}, errors.New("track not found")
	}
	for _, existing := range pl.TrackIDs {
		if existing == trackID {
			return pl, nil
		}
	}
	pl.TrackIDs = append(pl.TrackIDs, trackID)
	pl.UpdatedAt = time.Now().UTC()
	a.playlists[idx] = pl
	a.persistLocked()
	a.emitStateLocked(a.snapshotLocked())
	logEvent("playlist_track_added", "playlist_id", playlistID, "track_id", trackID, "track_count", len(pl.TrackIDs))
	return pl, nil
}

func (a *App) RemoveTrackFromPlaylist(playlistID, trackID string) (Playlist, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	pl, idx, ok := a.playlistByIDLocked(playlistID)
	if !ok {
		logEvent("playlist_remove_track_failed", "playlist_id", playlistID, "track_id", trackID, "reason", "playlist_not_found")
		return Playlist{}, errors.New("playlist not found")
	}
	nextIDs := make([]string, 0, len(pl.TrackIDs))
	removed := false
	for _, existing := range pl.TrackIDs {
		if existing == trackID {
			removed = true
			continue
		}
		nextIDs = append(nextIDs, existing)
	}
	if !removed {
		return pl, nil
	}
	pl.TrackIDs = nextIDs
	pl.UpdatedAt = time.Now().UTC()
	a.playlists[idx] = pl
	a.persistLocked()
	a.emitStateLocked(a.snapshotLocked())
	logEvent("playlist_track_removed", "playlist_id", playlistID, "track_id", trackID, "track_count", len(pl.TrackIDs))
	return pl, nil
}

func (a *App) MovePlaylistTrack(playlistID, trackID string, delta int) (Playlist, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	pl, idx, ok := a.playlistByIDLocked(playlistID)
	if !ok {
		logEvent("playlist_move_track_failed", "playlist_id", playlistID, "track_id", trackID, "reason", "playlist_not_found")
		return Playlist{}, errors.New("playlist not found")
	}
	if delta == 0 || len(pl.TrackIDs) < 2 {
		return pl, nil
	}
	position := -1
	for i, existing := range pl.TrackIDs {
		if existing == trackID {
			position = i
			break
		}
	}
	if position < 0 {
		logEvent("playlist_move_track_failed", "playlist_id", playlistID, "track_id", trackID, "reason", "track_not_in_playlist")
		return pl, nil
	}
	target := position + delta
	if target < 0 || target > len(pl.TrackIDs) {
		return pl, nil
	}
	nextIDs := append([]string(nil), pl.TrackIDs...)
	item := nextIDs[position]
	nextIDs = append(nextIDs[:position], nextIDs[position+1:]...)
	if target > position {
		target--
	}
	if target < 0 {
		target = 0
	}
	if target > len(nextIDs) {
		target = len(nextIDs)
	}
	nextIDs = append(nextIDs[:target], append([]string{item}, nextIDs[target:]...)...)
	pl.TrackIDs = nextIDs
	pl.UpdatedAt = time.Now().UTC()
	a.playlists[idx] = pl
	a.persistLocked()
	a.emitStateLocked(a.snapshotLocked())
	logEvent("playlist_track_moved", "playlist_id", playlistID, "track_id", trackID, "from_index", position, "to_index", target, "track_count", len(pl.TrackIDs))
	return pl, nil
}

func (a *App) playlistByIDLocked(playlistID string) (Playlist, int, bool) {
	for i := range a.playlists {
		if a.playlists[i].ID == playlistID {
			return a.playlists[i], i, true
		}
	}
	return Playlist{}, -1, false
}

func (a *App) rebuildTrackIndexLocked() {
	a.trackIndex = map[string]TrackRecord{}
	for _, track := range a.catalog.Tracks {
		a.trackIndex[track.ID] = a.trackWithArtworkMediaURL(track)
	}
	if a.player != nil {
		a.player.setLookup(a.trackIndex)
	}
}

func (a *App) persistLocked() {
	_ = a.store.saveSettings(a.settings)
	_ = a.store.saveCatalog(a.catalog)
	_ = a.store.saveLibraryCache(a.catalog)
	_ = a.store.savePlaylists(playlistFile{Version: playlistSchemaVersion, Playlists: append([]Playlist(nil), a.playlists...)})
	_ = a.store.saveImportHistory(importHistoryFile{Version: importHistorySchemaVersion, Entries: append([]ImportHistoryEntry(nil), a.importHistory...)})
}

func (a *App) updateJob(jobID string, mutate func(*Job)) Job {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.jobs {
		if a.jobs[i].ID == jobID {
			parentID := a.jobs[i].ParentJobID
			mutate(&a.jobs[i])
			a.jobs[i].StageStatuses = jobStageStatusesFor(a.jobs[i])
			a.syncCatalogJobLocked(a.jobs[i])
			if parentID != "" {
				a.refreshPlaylistBatchLocked(parentID)
			}
			a.persistLocked()
			a.emitStateLocked(a.snapshotLocked())
			return a.jobs[i]
		}
	}
	return Job{}
}

func (a *App) finishJob(jobID, status, detail string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.jobs {
		if a.jobs[i].ID == jobID {
			parentID := a.jobs[i].ParentJobID
			a.jobs[i].Status = status
			a.jobs[i].Detail = detail
			if status == "completed" {
				a.jobs[i].DownloadProgress = 100
				a.jobs[i].MetadataProgress = 100
				a.jobs[i].LyricsProgress = 100
			}
			a.jobs[i].StageStatuses = jobStageStatusesFor(a.jobs[i])
			a.jobs[i].FinishedAt = time.Now().UTC()
			a.syncCatalogJobLocked(a.jobs[i])
			if parentID != "" {
				a.refreshPlaylistBatchLocked(parentID)
			}
			a.persistLocked()
			a.logQueueStateLocked("queue_state_changed", "reason", "job_finished", "job_id", jobID, "status", status)
			a.emitStateLocked(a.snapshotLocked())
			a.requestWorkerPoolReconcile()
			return
		}
	}
}

func (a *App) failJob(jobID string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.jobs {
		if a.jobs[i].ID == jobID {
			log.Printf("job failed: %s: %v", jobID, err)
			parentID := a.jobs[i].ParentJobID
			a.jobs[i].Status = "failed"
			a.jobs[i].Error = err.Error()
			a.jobs[i].Detail = fmt.Sprintf("Import failed: %s", err.Error())
			a.jobs[i].StageStatuses = jobStageStatusesFor(a.jobs[i])
			a.jobs[i].FinishedAt = time.Now().UTC()
			a.syncCatalogJobLocked(a.jobs[i])
			if parentID != "" {
				a.refreshPlaylistBatchLocked(parentID)
			}
			a.persistLocked()
			a.logQueueStateLocked("queue_state_changed", "reason", "job_failed", "job_id", jobID, "error", err.Error())
			a.emitStateLocked(a.snapshotLocked())
			a.requestWorkerPoolReconcile()
			return
		}
	}
}

func (a *App) setJobProgress(jobID string, download, metadata, lyrics int) {
	download = clampProgress(download)
	metadata = clampProgress(metadata)
	lyrics = clampProgress(lyrics)

	a.mu.Lock()
	defer a.mu.Unlock()
	changed := false
	for i := range a.jobs {
		if a.jobs[i].ID != jobID {
			continue
		}
		parentID := a.jobs[i].ParentJobID
		if a.jobs[i].DownloadProgress != download {
			a.jobs[i].DownloadProgress = download
			changed = true
		}
		if a.jobs[i].MetadataProgress != metadata {
			a.jobs[i].MetadataProgress = metadata
			changed = true
		}
		if a.jobs[i].LyricsProgress != lyrics {
			a.jobs[i].LyricsProgress = lyrics
			changed = true
		}
		if changed {
			a.jobs[i].StageStatuses = jobStageStatusesFor(a.jobs[i])
			a.syncCatalogJobLocked(a.jobs[i])
			if parentID != "" {
				a.refreshPlaylistBatchLocked(parentID)
			}
			a.persistLocked()
			a.logQueueStateLocked("queue_progress_changed", "job_id", jobID, "download", download, "metadata", metadata, "lyrics", lyrics)
			a.emitJobProgressLocked(a.jobs[i])
		}
		return
	}
}

func (a *App) setJobResult(jobID, title, artist, album string) {
	title = strings.TrimSpace(title)
	artist = strings.TrimSpace(artist)
	album = strings.TrimSpace(album)

	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.jobs {
		if a.jobs[i].ID != jobID {
			continue
		}
		parentID := a.jobs[i].ParentJobID
		changed := false
		if a.jobs[i].ResultTitle != title {
			a.jobs[i].ResultTitle = title
			changed = true
		}
		if a.jobs[i].ResultArtist != artist {
			a.jobs[i].ResultArtist = artist
			changed = true
		}
		if a.jobs[i].ResultAlbum != album {
			a.jobs[i].ResultAlbum = album
			changed = true
		}
		if changed {
			a.jobs[i].StageStatuses = jobStageStatusesFor(a.jobs[i])
			a.syncCatalogJobLocked(a.jobs[i])
			if parentID != "" {
				a.refreshPlaylistBatchLocked(parentID)
			}
			a.persistLocked()
			a.logQueueStateLocked("queue_result_changed", "job_id", jobID, "title", title, "artist", artist, "album", album)
			a.requestStateEmit()
		}
		return
	}
}

func (a *App) RetryFailedPlaylistItems(rootID string) (AppState, error) {
	a.mu.Lock()
	rootIndex := -1
	for i := range a.jobs {
		if a.jobs[i].ID == rootID {
			rootIndex = i
			break
		}
	}
	if rootIndex < 0 {
		a.mu.Unlock()
		return AppState{}, fmt.Errorf("playlist job %s not found", rootID)
	}
	root := a.jobs[rootIndex]
	if root.Kind != "playlist" || root.ParentJobID != "" {
		a.mu.Unlock()
		return AppState{}, fmt.Errorf("job %s is not a playlist root", rootID)
	}

	failedChildren := a.playlistFailedChildrenLocked(rootID)
	if len(failedChildren) == 0 {
		state := a.snapshotLocked()
		a.mu.Unlock()
		return state, nil
	}

	now := time.Now().UTC()
	wasRunning := root.Status == "running"
	root.Status = "running"
	root.Detail = fmt.Sprintf("This is a playlist. Processing time extended. Retrying %d failed item(s).", len(failedChildren))
	root.Error = ""
	a.queueFrozen = false
	if !wasRunning {
		root.StartedAt = now
	}
	root.FinishedAt = time.Time{}
	a.jobs[rootIndex] = root
	a.syncCatalogJobLocked(root)

	for _, child := range failedChildren {
		insertBefore := ""
		for _, queuedID := range a.jobQueue {
			for _, queuedJob := range a.jobs {
				if queuedJob.ID != queuedID || queuedJob.ParentJobID != rootID || queuedJob.Status != "queued" {
					continue
				}
				if queuedJob.PlaylistIndex > child.PlaylistIndex {
					insertBefore = queuedJob.ID
					break
				}
			}
			if insertBefore != "" {
				break
			}
		}
		for i := range a.jobs {
			if a.jobs[i].ID != child.ID {
				continue
			}
			a.jobs[i].Status = "queued"
			a.jobs[i].Detail = "Waiting for worker"
			a.jobs[i].Error = ""
			a.jobs[i].StartedAt = time.Time{}
			a.jobs[i].FinishedAt = time.Time{}
			a.jobs[i].DownloadProgress = 0
			a.jobs[i].MetadataProgress = 0
			a.jobs[i].LyricsProgress = 0
			a.jobs[i].ResultTitle = ""
			a.jobs[i].ResultArtist = ""
			a.jobs[i].ResultAlbum = ""
			if insertBefore != "" {
				a.enqueueJobBeforeLocked(a.jobs[i].ID, insertBefore)
			} else {
				a.enqueueJobLocked(a.jobs[i].ID)
			}
			a.syncCatalogJobLocked(a.jobs[i])
			break
		}
	}

	a.refreshPlaylistBatchLocked(rootID)
	a.persistLocked()
	a.logQueueStateLocked("queue_state_changed", "reason", "playlist_retry_failed", "root_job_id", rootID, "failed_children", len(failedChildren))
	state := a.snapshotLocked()
	a.mu.Unlock()
	a.requestWorkerPoolReconcile()
	a.emitStateLocked(state)
	return state, nil
}

func clampProgress(value int) int {
	switch {
	case value < 0:
		return 0
	case value > 100:
		return 100
	default:
		return value
	}
}

func (a *App) syncCatalogJobLocked(job Job) {
	for i := range a.catalog.Jobs {
		if a.catalog.Jobs[i].ID == job.ID {
			a.catalog.Jobs[i] = job
			return
		}
	}
	a.catalog.Jobs = append(a.catalog.Jobs, job)
}

func (a *App) playlistChildrenLocked(rootID string) []Job {
	children := make([]Job, 0)
	for _, job := range a.jobs {
		if job.ParentJobID == rootID {
			children = append(children, job)
		}
	}
	sort.SliceStable(children, func(i, j int) bool {
		if children[i].PlaylistIndex != children[j].PlaylistIndex {
			return children[i].PlaylistIndex < children[j].PlaylistIndex
		}
		return children[i].CreatedAt.Before(children[j].CreatedAt)
	})
	return children
}

func (a *App) playlistFailedChildrenLocked(rootID string) []Job {
	children := a.playlistChildrenLocked(rootID)
	failed := make([]Job, 0)
	for _, child := range children {
		if child.Status == "failed" {
			failed = append(failed, child)
		}
	}
	return failed
}

func (a *App) pausePlaylistChildrenLocked(rootID string) {
	now := time.Now().UTC()
	for i := range a.jobs {
		if a.jobs[i].ParentJobID != rootID || a.jobs[i].Status != "running" {
			continue
		}
		if cancel := a.jobCancels[a.jobs[i].ID]; cancel != nil {
			cancel()
			delete(a.jobCancels, a.jobs[i].ID)
		}
		a.jobs[i].Status = "stopped"
		a.jobs[i].Detail = "Stopped by user"
		a.jobs[i].Error = ""
		a.jobs[i].FinishedAt = now
		a.syncCatalogJobLocked(a.jobs[i])
	}
	a.refreshPlaylistBatchLocked(rootID)
}

func (a *App) stopPlaylistChildrenLocked(rootID string) {
	now := time.Now().UTC()
	for i := range a.jobs {
		if a.jobs[i].ParentJobID != rootID {
			continue
		}
		if a.jobs[i].Status == "queued" {
			a.removeQueuedJobLocked(a.jobs[i].ID)
		}
		if a.jobs[i].Status != "running" && a.jobs[i].Status != "queued" {
			continue
		}
		if cancel := a.jobCancels[a.jobs[i].ID]; cancel != nil {
			cancel()
			delete(a.jobCancels, a.jobs[i].ID)
		}
		a.jobs[i].Status = "stopped"
		a.jobs[i].Detail = "Stopped by user"
		a.jobs[i].Error = ""
		a.jobs[i].FinishedAt = now
		a.syncCatalogJobLocked(a.jobs[i])
	}
	a.refreshPlaylistBatchLocked(rootID)
}

func (a *App) resumePlaylistChildrenLocked(rootID string) {
	changed := false
	for i := range a.jobs {
		if a.jobs[i].ParentJobID != rootID || a.jobs[i].Status != "stopped" {
			continue
		}
		a.jobs[i].Status = "queued"
		a.jobs[i].Detail = "Waiting for worker"
		a.jobs[i].Error = ""
		a.jobs[i].FinishedAt = time.Time{}
		a.enqueueJobLocked(a.jobs[i].ID)
		a.syncCatalogJobLocked(a.jobs[i])
		changed = true
	}
	if changed {
		a.refreshPlaylistBatchLocked(rootID)
		a.persistLocked()
	}
}

func (a *App) deletePlaylistChildrenLocked(rootID string) {
	var cancels []context.CancelFunc
	filtered := make([]Job, 0, len(a.jobs))
	removedIDs := make(map[string]struct{})
	changed := false
	for _, job := range a.jobs {
		if job.ParentJobID != rootID {
			filtered = append(filtered, job)
			continue
		}
		log.Printf("deleting playlist child: %s (%s)", job.ID, job.Status)
		removedIDs[job.ID] = struct{}{}
		if cancel := a.jobCancels[job.ID]; cancel != nil {
			cancels = append(cancels, cancel)
			delete(a.jobCancels, job.ID)
		}
		changed = true
	}
	if !changed {
		return
	}
	a.jobs = filtered
	a.catalog.Jobs = append([]Job(nil), filtered...)
	if len(removedIDs) > 0 && len(a.jobQueue) > 0 {
		filteredQueue := a.jobQueue[:0]
		for _, jobID := range a.jobQueue {
			if _, removed := removedIDs[jobID]; removed {
				continue
			}
			filteredQueue = append(filteredQueue, jobID)
		}
		a.jobQueue = append([]string(nil), filteredQueue...)
	}
	for _, cancel := range cancels {
		cancel()
	}
}

func (a *App) refreshPlaylistBatchLocked(rootID string) bool {
	rootIndex := -1
	for i := range a.jobs {
		if a.jobs[i].ID == rootID {
			rootIndex = i
			break
		}
	}
	if rootIndex < 0 {
		return false
	}
	root := a.jobs[rootIndex]
	if root.Kind != "playlist" || root.ParentJobID != "" {
		return false
	}
	children := a.playlistChildrenLocked(rootID)
	total := len(children)
	var (
		processed int
		failed    int
		running   *Job
		queued    *Job
	)
	for i := range children {
		child := children[i]
		switch child.Status {
		case "failed":
			failed++
			processed++
		case "completed", "stopped":
			processed++
		case "running":
			if running == nil {
				current := child
				running = &current
			}
		case "queued":
			if queued == nil {
				current := child
				queued = &current
			}
		}
	}
	current := running
	if current == nil {
		current = queued
	}
	root.PlaylistTotalItems = total
	root.PlaylistProcessedItems = processed
	root.PlaylistFailedItems = failed
	if current != nil {
		root.PlaylistCurrentIndex = current.PlaylistIndex
		root.PlaylistCurrentTitle = firstNonEmpty(current.PlaylistItemTitle, current.ResultTitle, current.Input)
	} else {
		root.PlaylistCurrentIndex = 0
		root.PlaylistCurrentTitle = ""
	}
	if total == 0 {
		root.Status = "completed"
		root.FinishedAt = time.Now().UTC()
		root.Detail = "Playlist import complete: no importable items found."
	} else if root.Status == "stopped" {
		if processed > 0 {
			root.Detail = fmt.Sprintf("This is a playlist. Processing time extended. Paused at %d/%d items.", processed, total)
		} else {
			root.Detail = "This is a playlist. Processing time extended."
		}
	} else if running != nil || queued != nil {
		root.Status = "running"
		if current != nil && root.PlaylistCurrentTitle != "" {
			root.Detail = fmt.Sprintf("This is a playlist. Processing time extended. Processing item %d/%d: %s", current.PlaylistIndex, total, root.PlaylistCurrentTitle)
		} else {
			root.Detail = fmt.Sprintf("This is a playlist. Processing time extended. %d/%d items complete.", processed, total)
		}
	} else {
		root.Status = "completed"
		root.FinishedAt = time.Now().UTC()
		if failed > 0 {
			root.Detail = fmt.Sprintf("Playlist import complete: %d/%d succeeded, %d failed.", total-failed, total, failed)
		} else {
			root.Detail = fmt.Sprintf("Playlist import complete: %d/%d items imported.", total, total)
		}
	}
	a.jobs[rootIndex] = root
	a.syncCatalogJobLocked(root)
	logEvent("playlist_queue_summary", "root_job_id", root.ID, "status", root.Status, "total", root.PlaylistTotalItems, "processed", root.PlaylistProcessedItems, "failed", root.PlaylistFailedItems, "current_index", root.PlaylistCurrentIndex, "current_title", root.PlaylistCurrentTitle)
	return root.Status == "completed"
}

func (a *App) finalizePlaylistRoot(rootID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.refreshPlaylistBatchLocked(rootID) {
		return
	}
	a.persistLocked()
	a.emitStateLocked(a.snapshotLocked())
}

func (a *App) enqueueJobLocked(jobID string) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return
	}
	for _, existing := range a.jobQueue {
		if existing == jobID {
			return
		}
	}
	a.jobQueue = append(a.jobQueue, jobID)
}

func (a *App) enqueueJobBeforeLocked(jobID, beforeJobID string) {
	jobID = strings.TrimSpace(jobID)
	beforeJobID = strings.TrimSpace(beforeJobID)
	if jobID == "" {
		return
	}
	for _, existing := range a.jobQueue {
		if existing == jobID {
			return
		}
	}
	if beforeJobID == "" {
		a.jobQueue = append(a.jobQueue, jobID)
		return
	}
	nextQueue := make([]string, 0, len(a.jobQueue)+1)
	inserted := false
	for _, existing := range a.jobQueue {
		if !inserted && existing == beforeJobID {
			nextQueue = append(nextQueue, jobID)
			inserted = true
		}
		nextQueue = append(nextQueue, existing)
	}
	if !inserted {
		nextQueue = append(nextQueue, jobID)
	}
	a.jobQueue = nextQueue
}

func (a *App) removeQueuedJobLocked(jobID string) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" || len(a.jobQueue) == 0 {
		return
	}
	filtered := a.jobQueue[:0]
	for _, existing := range a.jobQueue {
		if existing == jobID {
			continue
		}
		filtered = append(filtered, existing)
	}
	a.jobQueue = append([]string(nil), filtered...)
}

func (a *App) emitState() {
	a.mu.Lock()
	state := a.snapshotLocked()
	a.mu.Unlock()
	a.emitStateLocked(state)
}

func (a *App) emitStateLocked(state AppState) {
	if a.wailsApp == nil {
		return
	}
	if payload, err := json.Marshal(state); err == nil {
		logEvent("state_event_emitted", "event", "melodex:state", "payload_bytes", len(payload), "track_count", len(state.LibraryTracks), "job_count", len(state.Jobs))
	}
	a.wailsApp.Event.Emit("melodex:state", state)
}

func (a *App) emitPlaybackState(state PlaybackState) {
	if a.wailsApp == nil {
		return
	}
	if payload, err := json.Marshal(state); err == nil {
		logEvent("playback_event_emitted", "event", "melodex:playback", "payload_bytes", len(payload))
	}
	a.wailsApp.Event.Emit("melodex:playback", state)
}

func (a *App) emitJobProgressLocked(job Job) {
	if a.wailsApp == nil {
		return
	}
	payload := JobProgressEvent{
		JobID:             job.ID,
		DownloadProgress:  job.DownloadProgress,
		MetadataProgress:  job.MetadataProgress,
		LyricsProgress:    job.LyricsProgress,
		Status:            job.Status,
		Detail:            job.Detail,
		StageStatuses:     job.StageStatuses,
		ParentJobID:       job.ParentJobID,
		PlaylistTotal:     job.PlaylistTotalItems,
		PlaylistProcessed: job.PlaylistProcessedItems,
		PlaylistFailed:    job.PlaylistFailedItems,
	}
	if serialized, err := json.Marshal(payload); err == nil {
		logEvent("job_progress_event_emitted", "event", "melodex:job-progress", "job_id", job.ID, "payload_bytes", len(serialized))
	}
	a.wailsApp.Event.Emit("melodex:job-progress", payload)
}

func (a *App) requestStateEmit() {
	a.stateEmitMu.Lock()
	defer a.stateEmitMu.Unlock()
	if a.stateEmitTimer != nil {
		return
	}
	a.stateEmitTimer = time.AfterFunc(120*time.Millisecond, func() {
		a.stateEmitMu.Lock()
		a.stateEmitTimer = nil
		a.stateEmitMu.Unlock()
		a.emitState()
	})
}
