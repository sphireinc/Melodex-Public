package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type diagnosticsBundleManifest struct {
	CreatedAt       time.Time                      `json:"createdAt"`
	AppState        AppState                       `json:"appState"`
	BuildInfo       BuildInfo                      `json:"buildInfo"`
	UpdateInfo      UpdateInfo                     `json:"updateInfo"`
	RootInfo        RootInfo                       `json:"rootInfo"`
	ToolVersions    map[string]string              `json:"toolVersions,omitempty"`
	ToolReadiness   map[string]toolReadinessResult `json:"toolReadiness,omitempty"`
	MediaInventory  diagnosticsMediaInventory      `json:"mediaInventory"`
	CommandFailures []commandFailureRecord         `json:"commandFailures,omitempty"`
	LogPath         string                         `json:"logPath,omitempty"`
	Files           []string                       `json:"files"`
	Omissions       []diagnosticsFileOmission      `json:"omissions,omitempty"`
	Settings        storedSettings                 `json:"settings"`
}

type diagnosticsMediaKindSummary struct {
	References  int `json:"references"`
	Existing    int `json:"existing"`
	Missing     int `json:"missing"`
	Invalid     int `json:"invalid"`
	Unsupported int `json:"unsupported"`
}

type diagnosticsMediaInventory struct {
	ReferencesChecked int                                    `json:"referencesChecked"`
	ExistingFiles     int                                    `json:"existingFiles"`
	MissingFiles      int                                    `json:"missingFiles"`
	InvalidFiles      int                                    `json:"invalidFiles"`
	UnsupportedFiles  int                                    `json:"unsupportedFiles"`
	MIMECounts        map[string]int                         `json:"mimeCounts,omitempty"`
	ByKind            map[string]diagnosticsMediaKindSummary `json:"byKind,omitempty"`
	Truncated         bool                                   `json:"truncated"`
}

const (
	maxDiagnosticsMediaReferences = 10000
	maxDiagnosticsMediaMIMETypes  = 32
)

func collectDiagnosticsMediaInventory(state AppState, rootInfo RootInfo) diagnosticsMediaInventory {
	inventory := diagnosticsMediaInventory{
		MIMECounts: map[string]int{},
		ByKind:     map[string]diagnosticsMediaKindSummary{},
	}
	allowedRoots := []string{rootInfo.LibraryRoot, rootInfo.AppDataDir, rootInfo.IncomingDir, rootInfo.CacheDir}
	seen := make(map[string]struct{})

	addReference := func(kind, path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		normalized := filepath.Clean(path)
		if absolute, err := filepath.Abs(normalized); err == nil {
			normalized = absolute
		}
		key := kind + "\x00" + normalized
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		if inventory.ReferencesChecked >= maxDiagnosticsMediaReferences {
			inventory.Truncated = true
			return
		}

		inventory.ReferencesChecked++
		summary := inventory.ByKind[kind]
		summary.References++
		resolved, err := validateMediaReadPath(path, allowedRoots)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				summary.Missing++
				inventory.MissingFiles++
			} else {
				summary.Invalid++
				inventory.InvalidFiles++
			}
			inventory.ByKind[kind] = summary
			return
		}
		info, err := os.Stat(resolved)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				summary.Missing++
				inventory.MissingFiles++
			} else {
				summary.Invalid++
				inventory.InvalidFiles++
			}
			inventory.ByKind[kind] = summary
			return
		}
		if !info.Mode().IsRegular() {
			summary.Invalid++
			inventory.InvalidFiles++
			inventory.ByKind[kind] = summary
			return
		}

		mimeType := mediaTypeForPath(resolved)
		addDiagnosticsMIMECount(inventory.MIMECounts, mimeType)
		summary.Existing++
		inventory.ExistingFiles++
		if !supportedMediaPath(resolved) {
			summary.Unsupported++
			inventory.UnsupportedFiles++
		}
		inventory.ByKind[kind] = summary
	}

	for _, tracks := range [][]TrackRecord{state.LibraryTracks, state.RecentTracks} {
		for _, track := range tracks {
			addReference("audio", track.AudioPath)
			addReference("video", track.VideoPath)
			addReference("artwork", track.ArtworkPath)
		}
	}
	return inventory
}

func addDiagnosticsMIMECount(counts map[string]int, mimeType string) {
	if strings.TrimSpace(mimeType) == "" {
		mimeType = "application/octet-stream"
	}
	if _, exists := counts[mimeType]; exists {
		counts[mimeType]++
		return
	}
	if len(counts) >= maxDiagnosticsMediaMIMETypes-1 {
		counts["other"]++
		return
	}
	counts[mimeType] = 1
}

type diagnosticsFileOmission struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

func defaultDiagnosticsInfo() DiagnosticsInfo {
	return DiagnosticsInfo{
		LogPath: currentLogPath(),
	}
}

func (a *App) ExportDiagnostics() (AppState, error) {
	a.mu.Lock()
	state := a.snapshotLocked()
	settings := a.settings
	buildInfo := a.buildInfo
	updateInfo := a.updateInfo
	rootInfo := a.info
	store := a.store
	logPath := currentLogPath()
	a.mu.Unlock()

	if store == nil {
		logEvent("diagnostics_export_failed", "reason", "store_unavailable")
		return state, fmt.Errorf("store unavailable")
	}

	logEvent("diagnostics_export_started", "log_path", logPath, "library_root", rootInfo.LibraryRoot)
	toolReadiness := a.collectToolReadiness()
	toolVersions := toolVersionStrings(toolReadiness)
	commandFailures := recentCommandFailuresSnapshot()
	exportPath, err := writeDiagnosticsBundleWithReadiness(store, state, settings, buildInfo, updateInfo, rootInfo, logPath, toolVersions, toolReadiness, commandFailures)

	a.mu.Lock()
	if err != nil {
		a.diagnostics.BundleStatus = "Failed"
		a.diagnostics.BundleMessage = err.Error()
	} else {
		a.diagnostics.LastExportPath = exportPath
		a.diagnostics.LastExportedAt = time.Now().UTC()
		a.diagnostics.LogPath = logPath
		a.diagnostics.BundleStatus = "Exported"
		a.diagnostics.BundleMessage = exportPath
	}
	if err != nil {
		logEvent("diagnostics_export_failed", "error", err.Error(), "log_path", logPath)
	} else {
		logEvent("diagnostics_export_completed", "bundle_path", exportPath, "log_path", logPath)
	}
	state = a.snapshotLocked()
	a.mu.Unlock()
	a.emitStateLocked(state)
	return state, err
}

func (a *App) collectToolVersions() map[string]string {
	return toolVersionStrings(a.collectToolReadiness())
}

func (a *App) collectToolReadiness() map[string]toolReadinessResult {
	readiness := map[string]toolReadinessResult{
		"ytDlp":  {ToolName: "yt-dlp", Status: toolReadinessMissing, Error: "executable not found"},
		"ffmpeg": {ToolName: "ffmpeg", Status: toolReadinessMissing, Error: "executable not found"},
	}
	if path := strings.TrimSpace(a.toolPath(a.settings.YTDLPPath, "yt-dlp")); path != "" {
		readiness["ytDlp"] = probeToolReadiness(a.appContext(), path, []string{"--version"}, "yt-dlp", toolProbeTimeout)
	}
	if path := strings.TrimSpace(a.toolPath(a.settings.FFmpegPath, "ffmpeg")); path != "" {
		readiness["ffmpeg"] = probeToolReadiness(a.appContext(), path, []string{"-version"}, "ffmpeg", toolProbeTimeout)
	}
	return readiness
}

func toolVersionStrings(readiness map[string]toolReadinessResult) map[string]string {
	versions := map[string]string{
		"go":   runtime.Version(),
		"os":   runtime.GOOS,
		"arch": runtime.GOARCH,
	}
	for key, result := range readiness {
		switch result.Status {
		case toolReadinessReady:
			versions[key] = firstLine(result.Version)
		case toolReadinessTimedOut:
			versions[key] = "timeout"
		case toolReadinessProbeFailed:
			versions[key] = "probe failed"
		default:
			versions[key] = "unavailable"
		}
	}
	return versions
}

func writeDiagnosticsBundle(store *Store, state AppState, settings storedSettings, buildInfo BuildInfo, updateInfo UpdateInfo, rootInfo RootInfo, logPath string, toolVersions map[string]string, commandFailures []commandFailureRecord) (string, error) {
	return writeDiagnosticsBundleWithReadiness(store, state, settings, buildInfo, updateInfo, rootInfo, logPath, toolVersions, nil, commandFailures)
}

func writeDiagnosticsBundleWithReadiness(store *Store, state AppState, settings storedSettings, buildInfo BuildInfo, updateInfo UpdateInfo, rootInfo RootInfo, logPath string, toolVersions map[string]string, toolReadiness map[string]toolReadinessResult, commandFailures []commandFailureRecord) (string, error) {
	if err := os.MkdirAll(filepath.Join(rootInfo.AppDataDir, "diagnostics"), 0o755); err != nil {
		return "", err
	}
	timestamp := time.Now().UTC().Format("20060102-150405")
	bundlePath := filepath.Join(rootInfo.AppDataDir, "diagnostics", fmt.Sprintf("melodex-diagnostics-%s.zip", timestamp))
	file, err := os.CreateTemp(filepath.Dir(bundlePath), "."+filepath.Base(bundlePath)+".tmp-*")
	if err != nil {
		return "", err
	}
	tmpPath := file.Name()
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(tmpPath)
	}

	zw := zip.NewWriter(file)

	addJSON := func(name string, value any) error {
		entry, err := zw.Create(name)
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(entry)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}

	files := []string{}
	omissions := []diagnosticsFileOmission{}
	mediaInventory := collectDiagnosticsMediaInventory(state, rootInfo)
	state = redactAppState(state)
	updateInfo = redactUpdateInfo(updateInfo)

	if err := addJSON("state.json", state); err != nil {
		return "", err
	}
	files = append(files, "state.json")

	if err := addJSON("build-info.json", buildInfo); err != nil {
		return "", err
	}
	files = append(files, "build-info.json")

	if err := addJSON("update-info.json", updateInfo); err != nil {
		return "", err
	}
	files = append(files, "update-info.json")

	if err := addJSON("settings.json", redactStoredSettings(settings)); err != nil {
		return "", err
	}
	files = append(files, "settings.json")

	if err := addJSON("tool-versions.json", toolVersions); err != nil {
		return "", err
	}
	files = append(files, "tool-versions.json")

	if err := addJSON("command-failures.json", commandFailures); err != nil {
		return "", err
	}
	files = append(files, "command-failures.json")

	optionalFiles := []struct {
		sourcePath string
		entryName  string
	}{
		{store.catalogPath, "catalog.json"},
		{store.libraryCachePath, "library-index.json"},
		{store.playlistsPath, "playlists.json"},
		{store.importHistoryPath, "imports.json"},
		{logPath, "logs/dev.log"},
	}
	for _, optional := range optionalFiles {
		if strings.TrimSpace(optional.sourcePath) == "" {
			continue
		}
		if err := addRedactedFileIfExists(zw, optional.sourcePath, optional.entryName); err == nil {
			files = append(files, optional.entryName)
		} else {
			omissions = append(omissions, diagnosticsFileOmission{Name: optional.entryName, Reason: diagnosticsOmissionReason(err)})
		}
	}
	if recent := recentLogs.Bytes(); len(recent) > 0 {
		entry, err := zw.Create("logs/recent.log")
		if err != nil {
			cleanup()
			return "", err
		}
		if _, err := io.WriteString(entry, redactSensitiveText(string(recent))); err != nil {
			cleanup()
			return "", err
		}
		files = append(files, "logs/recent.log")
	}

	manifest := diagnosticsBundleManifest{
		CreatedAt:       time.Now().UTC(),
		AppState:        state,
		BuildInfo:       buildInfo,
		UpdateInfo:      updateInfo,
		RootInfo:        rootInfo,
		ToolVersions:    toolVersions,
		ToolReadiness:   toolReadiness,
		MediaInventory:  mediaInventory,
		CommandFailures: commandFailures,
		LogPath:         logPath,
		Files:           append(append([]string(nil), files...), "manifest.json"),
		Omissions:       omissions,
		Settings:        redactStoredSettings(settings),
	}
	if err := addJSON("manifest.json", manifest); err != nil {
		cleanup()
		return "", err
	}
	files = append(files, "manifest.json")
	if err := zw.Close(); err != nil {
		cleanup()
		return "", err
	}
	if err := file.Sync(); err != nil {
		cleanup()
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := os.Rename(tmpPath, bundlePath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return bundlePath, nil
}

func diagnosticsOmissionReason(err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return "missing"
	}
	if errors.Is(err, os.ErrPermission) {
		return "unreadable"
	}
	if strings.Contains(err.Error(), "too large") {
		return "too large"
	}
	return "unreadable"
}

func addRedactedFileIfExists(zw *zip.Writer, sourcePath, entryName string) error {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return os.ErrNotExist
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}
	const maxDiagnosticsTextFileBytes = 16 * 1024 * 1024
	if info.Size() > maxDiagnosticsTextFileBytes {
		return fmt.Errorf("diagnostics source is too large: %s (%d bytes)", sourcePath, info.Size())
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	entry, err := zw.Create(entryName)
	if err != nil {
		return err
	}
	_, err = io.WriteString(entry, redactSensitiveText(string(data)))
	return err
}

func redactStoredSettings(settings storedSettings) storedSettings {
	settings.APIKey = ""
	settings.YTDLPCookiesPath = ""
	settings.YTDLPCookiesFromBrowser = ""
	settings.UpdateManifestURL = redactURLString(settings.UpdateManifestURL)
	return settings
}

func redactAppState(state AppState) AppState {
	state.Settings = redactPublicSettings(state.Settings)
	state.UpdateInfo = redactUpdateInfo(state.UpdateInfo)
	for i := range state.Jobs {
		state.Jobs[i].Input = redactDiagnosticString(state.Jobs[i].Input)
		state.Jobs[i].PlaylistURL = redactDiagnosticString(state.Jobs[i].PlaylistURL)
		state.Jobs[i].Error = redactDiagnosticString(state.Jobs[i].Error)
		state.Jobs[i].Detail = redactDiagnosticString(state.Jobs[i].Detail)
		state.Jobs[i].OutputBundle = redactDiagnosticString(state.Jobs[i].OutputBundle)
	}
	for i := range state.ImportHistory {
		state.ImportHistory[i].URL = redactDiagnosticString(state.ImportHistory[i].URL)
	}
	for i := range state.LibraryTracks {
		redactTrackRecord(&state.LibraryTracks[i])
	}
	for i := range state.RecentTracks {
		redactTrackRecord(&state.RecentTracks[i])
	}
	return state
}

func redactTrackRecord(track *TrackRecord) {
	if track == nil {
		return
	}
	track.SourceRef = redactDiagnosticString(track.SourceRef)
	track.SourceTitle = redactDiagnosticString(track.SourceTitle)
	track.SourceChannel = redactDiagnosticString(track.SourceChannel)
	track.VideoURL = redactDiagnosticString(track.VideoURL)
	track.ArtworkURL = redactDiagnosticString(track.ArtworkURL)
	track.ArtistLinks = redactMetadataLinks(track.ArtistLinks)
	track.AlbumLinks = redactMetadataLinks(track.AlbumLinks)
	track.SongLinks = redactMetadataLinks(track.SongLinks)
	track.Sources = redactDiagnosticStrings(track.Sources)
}

func redactMetadataLinks(links MetadataLinks) MetadataLinks {
	links.OfficialWebsite = redactDiagnosticString(links.OfficialWebsite)
	links.Wikipedia = redactDiagnosticString(links.Wikipedia)
	links.MusicBrainz = redactDiagnosticString(links.MusicBrainz)
	links.Genius = redactDiagnosticString(links.Genius)
	links.YouTube = redactDiagnosticString(links.YouTube)
	links.YouTubeMusic = redactDiagnosticString(links.YouTubeMusic)
	links.AppleMusic = redactDiagnosticString(links.AppleMusic)
	links.Spotify = redactDiagnosticString(links.Spotify)
	links.Instagram = redactDiagnosticString(links.Instagram)
	links.X = redactDiagnosticString(links.X)
	links.Facebook = redactDiagnosticString(links.Facebook)
	links.Bandcamp = redactDiagnosticString(links.Bandcamp)
	links.SoundCloud = redactDiagnosticString(links.SoundCloud)
	return links
}

func redactDiagnosticStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	redacted := make([]string, len(values))
	for i, value := range values {
		redacted[i] = redactDiagnosticString(value)
	}
	return redacted
}

func redactDiagnosticString(value string) string {
	return redactSensitiveText(strings.TrimSpace(value))
}

func redactPublicSettings(settings PublicSettings) PublicSettings {
	settings.YTDLPCookiesPath = ""
	settings.YTDLPCookiesFromBrowser = ""
	settings.UpdateManifestURL = redactURLString(settings.UpdateManifestURL)
	return settings
}

func redactUpdateInfo(info UpdateInfo) UpdateInfo {
	info.ManifestURL = redactURLString(info.ManifestURL)
	info.DownloadURL = redactURLString(info.DownloadURL)
	return info
}
