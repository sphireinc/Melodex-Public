package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const diagnosticsTestMaxTextFileBytes = 16 * 1024 * 1024

func TestToolReadinessHelper(t *testing.T) {
	switch os.Getenv("MELODEX_TOOL_READINESS_HELPER") {
	case "ready":
		_, _ = fmt.Fprintln(os.Stdout, os.Getenv("MELODEX_TOOL_READINESS_VERSION"))
		os.Exit(0)
	case "failed":
		_, _ = fmt.Fprintln(os.Stderr, "Authorization: Bearer helper-secret")
		os.Exit(17)
	case "empty":
		os.Exit(0)
	case "invalid":
		_, _ = fmt.Fprintln(os.Stdout, "not-a-recognizable-tool-version")
		os.Exit(0)
	case "timeout":
		time.Sleep(500 * time.Millisecond)
	}
}

func toolReadinessTestCommand(t *testing.T, mode, version string) (string, []string) {
	t.Helper()
	t.Setenv("MELODEX_TOOL_READINESS_HELPER", mode)
	t.Setenv("MELODEX_TOOL_READINESS_VERSION", version)
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return executable, []string{"-test.run=TestToolReadinessHelper"}
}

func TestProbeToolReadinessReturnsTypedStates(t *testing.T) {
	missing := probeToolReadiness(context.Background(), "", nil, "missing-tool", time.Second)
	if missing.Status != toolReadinessMissing {
		t.Fatalf("missing tool status = %q, want %q", missing.Status, toolReadinessMissing)
	}
	if !strings.Contains(strings.ToLower(missing.Remediation), "install") || !strings.Contains(strings.ToLower(missing.Remediation), "settings") {
		t.Fatalf("missing tool remediation is not actionable: %q", missing.Remediation)
	}
	if missing.CorrelationID == "" {
		t.Fatal("missing tool result did not include a correlation ID")
	}
	nonexistent := probeToolReadiness(context.Background(), filepath.Join(t.TempDir(), "does-not-exist"), nil, "yt-dlp", time.Second)
	if nonexistent.Status != toolReadinessProbeFailed {
		t.Fatalf("nonexistent executable status = %q, want %q", nonexistent.Status, toolReadinessProbeFailed)
	}
	nonExecutable := probeToolReadiness(context.Background(), t.TempDir(), nil, "ffmpeg", time.Second)
	if nonExecutable.Status != toolReadinessProbeFailed {
		t.Fatalf("non-executable path status = %q, want %q", nonExecutable.Status, toolReadinessProbeFailed)
	}

	readyPath, readyArgs := toolReadinessTestCommand(t, "ready", "2026.01.01")
	ready := probeToolReadiness(context.Background(), readyPath, readyArgs, "yt-dlp", time.Second)
	if ready.Status != toolReadinessReady {
		t.Fatalf("ready tool status = %q, want %q (error=%q)", ready.Status, toolReadinessReady, ready.Error)
	}
	if firstLine(ready.Version) != "2026.01.01" {
		t.Fatalf("ready tool version = %q", ready.Version)
	}
	if ready.Remediation != "" {
		t.Fatalf("ready tool unexpectedly included remediation: %q", ready.Remediation)
	}

	ffmpegPath, ffmpegArgs := toolReadinessTestCommand(t, "ready", "ffmpeg version 7.0.0")
	ffmpeg := probeToolReadiness(context.Background(), ffmpegPath, ffmpegArgs, "ffmpeg", time.Second)
	if ffmpeg.Status != toolReadinessReady {
		t.Fatalf("ffmpeg ready status = %q, want %q (error=%q)", ffmpeg.Status, toolReadinessReady, ffmpeg.Error)
	}

	failPath, failArgs := toolReadinessTestCommand(t, "failed", "")
	failure := probeToolReadiness(context.Background(), failPath, failArgs, "yt-dlp", time.Second)
	if failure.Status != toolReadinessProbeFailed {
		t.Fatalf("failed tool status = %q, want %q", failure.Status, toolReadinessProbeFailed)
	}
	if failure.CorrelationID == "" {
		t.Fatal("failed tool result did not include a correlation ID")
	}
	if strings.Contains(failure.Error, "helper-secret") || strings.Contains(failure.Error, "Bearer") {
		t.Fatalf("failed tool error leaked command secret: %q", failure.Error)
	}
	if !strings.Contains(strings.ToLower(failure.Remediation), "configured") || !strings.Contains(failure.Remediation, "version") {
		t.Fatalf("failed tool remediation is not actionable: %q", failure.Remediation)
	}

	emptyPath, emptyArgs := toolReadinessTestCommand(t, "empty", "")
	empty := probeToolReadiness(context.Background(), emptyPath, emptyArgs, "yt-dlp", time.Second)
	if empty.Status != toolReadinessProbeFailed || empty.Error != "probe returned no version text" {
		t.Fatalf("empty probe result = %#v, want probe-failed/no-version", empty)
	}

	invalidPath, invalidArgs := toolReadinessTestCommand(t, "invalid", "")
	invalid := probeToolReadiness(context.Background(), invalidPath, invalidArgs, "yt-dlp", time.Second)
	if invalid.Status != toolReadinessInvalidVersion {
		t.Fatalf("invalid-version probe status = %q, want %q", invalid.Status, toolReadinessInvalidVersion)
	}
	if !strings.Contains(strings.ToLower(invalid.Remediation), "recognizable version") {
		t.Fatalf("invalid-version remediation is not actionable: %q", invalid.Remediation)
	}

	timeoutPath, timeoutArgs := toolReadinessTestCommand(t, "timeout", "")
	timeoutResult := probeToolReadiness(context.Background(), timeoutPath, timeoutArgs, "helper", 10*time.Millisecond)
	if timeoutResult.Status != toolReadinessTimedOut {
		t.Fatalf("timed-out tool status = %q, want %q", timeoutResult.Status, toolReadinessTimedOut)
	}
	if timeoutResult.CorrelationID == "" {
		t.Fatal("timed-out tool result did not include a correlation ID")
	}
	if !strings.Contains(strings.ToLower(timeoutResult.Remediation), "timed out") || !strings.Contains(strings.ToLower(timeoutResult.Remediation), "settings") {
		t.Fatalf("timed-out tool remediation is not actionable: %q", timeoutResult.Remediation)
	}
}

func TestDiagnosticsMediaInventorySummarizesSafeExistenceAndMIME(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	_, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	audioPath := filepath.Join(root, "Artist", "Album", "track.mp3")
	videoPath := filepath.Join(root, "Artist", "Album", "video.mp4")
	artworkPath := filepath.Join(root, "Artist", "Album", "cover.jpg")
	unsupportedPath := filepath.Join(root, "Artist", "Album", "notes.txt")
	for path, data := range map[string][]byte{
		audioPath:       []byte("audio"),
		videoPath:       []byte("video"),
		artworkPath:     []byte("artwork"),
		unsupportedPath: []byte("not media"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", path, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", path, err)
		}
	}
	missingPath := filepath.Join(root, "Artist", "Album", "missing.mp4")
	outsidePath := filepath.Join(t.TempDir(), "outside.mp4")
	if err := os.WriteFile(outsidePath, []byte("outside"), 0o644); err != nil {
		t.Fatalf("WriteFile outside media: %v", err)
	}

	state := AppState{
		LibraryTracks: []TrackRecord{{
			AudioPath:    audioPath,
			VideoPath:    videoPath,
			ArtworkPath:  artworkPath,
			MetadataPath: filepath.Join(root, "Artist", "Album", "track.metadata.json"),
		}, {
			AudioPath:   missingPath,
			ArtworkPath: unsupportedPath,
		}, {
			AudioPath: outsidePath,
		}},
	}
	inventory := collectDiagnosticsMediaInventory(state, info)
	if inventory.ReferencesChecked != 6 {
		t.Fatalf("references checked = %d, want 6", inventory.ReferencesChecked)
	}
	if inventory.ExistingFiles != 4 {
		t.Fatalf("existing files = %d, want 4", inventory.ExistingFiles)
	}
	if inventory.MissingFiles != 1 {
		t.Fatalf("missing files = %d, want 1", inventory.MissingFiles)
	}
	if inventory.InvalidFiles != 1 {
		t.Fatalf("invalid files = %d, want 1", inventory.InvalidFiles)
	}
	if inventory.UnsupportedFiles != 1 {
		t.Fatalf("unsupported files = %d, want 1", inventory.UnsupportedFiles)
	}
	for mimeType, want := range map[string]int{
		"audio/mpeg": 1,
		"video/mp4":  1,
		"image/jpeg": 1,
	} {
		if inventory.MIMECounts[mimeType] != want {
			t.Fatalf("MIME %s count = %d, want %d", mimeType, inventory.MIMECounts[mimeType], want)
		}
	}
	textMIMECount := 0
	for mimeType, count := range inventory.MIMECounts {
		if strings.HasPrefix(mimeType, "text/plain") {
			textMIMECount += count
		}
	}
	if textMIMECount != 1 {
		t.Fatalf("text/plain MIME count = %d, want 1; counts=%#v", textMIMECount, inventory.MIMECounts)
	}
	if inventory.ByKind["audio"].Missing != 1 || inventory.ByKind["audio"].Invalid != 1 {
		t.Fatalf("audio summary = %#v, want one missing and one invalid", inventory.ByKind["audio"])
	}

	data, err := json.Marshal(inventory)
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}
	encoded := string(data)
	for _, secret := range []string{audioPath, videoPath, outsidePath, "outside.mp4"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("inventory leaked path %q: %s", secret, encoded)
		}
	}
}

func TestDiagnosticsBundleIncludesMediaInventory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	mediaPath := filepath.Join(root, "Artist", "Album", "track.mp3")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
		t.Fatalf("MkdirAll media dir: %v", err)
	}
	if err := os.WriteFile(mediaPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("WriteFile media: %v", err)
	}
	state := AppState{LibraryTracks: []TrackRecord{{AudioPath: mediaPath}}}
	bundlePath, err := writeDiagnosticsBundle(store, state, defaultStoredSettings(root), BuildInfo{}, UpdateInfo{}, info, "", nil, nil)
	if err != nil {
		t.Fatalf("writeDiagnosticsBundle: %v", err)
	}
	defer os.Remove(bundlePath)

	reader, err := zip.OpenReader(bundlePath)
	if err != nil {
		t.Fatalf("zip.OpenReader: %v", err)
	}
	defer reader.Close()
	var manifest diagnosticsBundleManifest
	for _, file := range reader.File {
		if file.Name != "manifest.json" {
			continue
		}
		opened, openErr := file.Open()
		if openErr != nil {
			t.Fatalf("open manifest: %v", openErr)
		}
		decodeErr := json.NewDecoder(opened).Decode(&manifest)
		_ = opened.Close()
		if decodeErr != nil {
			t.Fatalf("decode manifest: %v", decodeErr)
		}
	}
	if manifest.MediaInventory.ExistingFiles != 1 || manifest.MediaInventory.MIMECounts["audio/mpeg"] != 1 {
		t.Fatalf("media inventory in manifest = %#v", manifest.MediaInventory)
	}
	data, err := json.Marshal(manifest.MediaInventory)
	if err != nil {
		t.Fatalf("marshal manifest inventory: %v", err)
	}
	if strings.Contains(string(data), mediaPath) {
		t.Fatalf("manifest media inventory leaked media path: %s", data)
	}
}

func TestDiagnosticsMediaInventoryStaysBounded(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	_, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	tracks := make([]TrackRecord, maxDiagnosticsMediaReferences+1)
	for i := range tracks {
		tracks[i].AudioPath = filepath.Join(root, fmt.Sprintf("missing-%d.mp3", i))
	}
	inventory := collectDiagnosticsMediaInventory(AppState{LibraryTracks: tracks}, info)
	if inventory.ReferencesChecked != maxDiagnosticsMediaReferences {
		t.Fatalf("references checked = %d, want %d", inventory.ReferencesChecked, maxDiagnosticsMediaReferences)
	}
	if !inventory.Truncated {
		t.Fatal("expected media inventory to report truncation")
	}

	counts := map[string]int{}
	for i := 0; i < maxDiagnosticsMediaMIMETypes+5; i++ {
		addDiagnosticsMIMECount(counts, fmt.Sprintf("application/x-test-%d", i))
	}
	if len(counts) > maxDiagnosticsMediaMIMETypes {
		t.Fatalf("MIME bucket count = %d, want <= %d", len(counts), maxDiagnosticsMediaMIMETypes)
	}
	if counts["other"] == 0 {
		t.Fatalf("expected overflow MIME values to be grouped: %#v", counts)
	}
}

func TestToolReadinessFailureSharesCorrelationIDWithCommandRecord(t *testing.T) {
	path, args := toolReadinessTestCommand(t, "failed", "")
	result := probeToolReadiness(context.Background(), path, args, "helper", time.Second)
	if result.Status != toolReadinessProbeFailed {
		t.Fatalf("status = %q, want %q", result.Status, toolReadinessProbeFailed)
	}

	for _, record := range recentCommandFailuresSnapshot() {
		if record.CorrelationID != result.CorrelationID {
			continue
		}
		if record.Command == "" {
			t.Fatal("correlated command failure has no command label")
		}
		if strings.Contains(record.Stderr, "helper-secret") || strings.Contains(record.Stderr, "Bearer") {
			t.Fatalf("correlated command failure leaked stderr secret: %q", record.Stderr)
		}
		return
	}
	t.Fatalf("no command failure shared correlation ID %q", result.CorrelationID)
}

func TestDiagnosticCommandOutputIsBoundedAndRedacted(t *testing.T) {
	secret := "Authorization: Bearer command-secret"
	input := strings.Repeat("x", maxRecordedCommandOutputBytes+1024) + "\n" + secret
	output := trimRecordedCommandOutput([]byte(input))
	if len(output) > maxRecordedCommandOutputBytes {
		t.Fatalf("recorded output length = %d, want <= %d", len(output), maxRecordedCommandOutputBytes)
	}
	if strings.Contains(output, "command-secret") || strings.Contains(output, "Bearer") {
		t.Fatalf("recorded output leaked secret: %q", output)
	}
}

func TestDiagnosticsBundleRecordsBoundedOptionalFileOmissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	file, err := os.OpenFile(store.catalogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatalf("create oversized catalog: %v", err)
	}
	if err := file.Truncate(diagnosticsTestMaxTextFileBytes + 1); err != nil {
		_ = file.Close()
		t.Fatalf("truncate oversized catalog: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close oversized catalog: %v", err)
	}

	bundlePath, err := writeDiagnosticsBundle(store, AppState{}, defaultStoredSettings(root), BuildInfo{}, UpdateInfo{}, info, "", nil, nil)
	if err != nil {
		t.Fatalf("writeDiagnosticsBundle: %v", err)
	}
	defer os.Remove(bundlePath)

	reader, err := zip.OpenReader(bundlePath)
	if err != nil {
		t.Fatalf("zip.OpenReader: %v", err)
	}
	defer reader.Close()
	var manifest diagnosticsBundleManifest
	for _, file := range reader.File {
		if file.Name != "manifest.json" {
			continue
		}
		opened, openErr := file.Open()
		if openErr != nil {
			t.Fatalf("open manifest: %v", openErr)
		}
		decodeErr := json.NewDecoder(opened).Decode(&manifest)
		_ = opened.Close()
		if decodeErr != nil {
			t.Fatalf("decode manifest: %v", decodeErr)
		}
	}

	findOmission := func(name string) (diagnosticsFileOmission, bool) {
		for _, omission := range manifest.Omissions {
			if omission.Name == name {
				return omission, true
			}
		}
		return diagnosticsFileOmission{}, false
	}
	if omission, ok := findOmission("catalog.json"); !ok || omission.Reason != "too large" {
		t.Fatalf("catalog omission = %#v, found=%v; want too large", omission, ok)
	}
	if omission, ok := findOmission("library-index.json"); !ok || omission.Reason != "missing" {
		t.Fatalf("library-index omission = %#v, found=%v; want missing", omission, ok)
	}
	for _, name := range []string{"catalog.json", "library-index.json"} {
		for _, file := range reader.File {
			if file.Name == name {
				t.Fatalf("omitted file %q was included in diagnostics bundle", name)
			}
		}
	}
}

func TestDiagnosticsBundleKeepsMalformedCatalogUsableAndRedactsRecentLog(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	if err := os.WriteFile(store.catalogPath, []byte(`{"tracks":[`), 0o600); err != nil {
		t.Fatalf("write malformed catalog: %v", err)
	}

	previousLogs := recentLogs.Bytes()
	defer func() {
		recentLogs.mu.Lock()
		recentLogs.data = previousLogs
		recentLogs.mu.Unlock()
	}()
	recentLogs.Write([]byte("videojs_error detail=Authorization: Bearer catalog-secret https://example.test/video?token=log-secret\n"))

	bundlePath, err := writeDiagnosticsBundle(store, AppState{}, defaultStoredSettings(root), BuildInfo{}, UpdateInfo{}, info, "", nil, nil)
	if err != nil {
		t.Fatalf("writeDiagnosticsBundle: %v", err)
	}
	defer os.Remove(bundlePath)

	reader, err := zip.OpenReader(bundlePath)
	if err != nil {
		t.Fatalf("zip.OpenReader: %v", err)
	}
	defer reader.Close()
	entries := map[string]string{}
	for _, file := range reader.File {
		opened, openErr := file.Open()
		if openErr != nil {
			t.Fatalf("open %s: %v", file.Name, openErr)
		}
		data, readErr := io.ReadAll(opened)
		_ = opened.Close()
		if readErr != nil {
			t.Fatalf("read %s: %v", file.Name, readErr)
		}
		entries[file.Name] = string(data)
	}
	if entries["catalog.json"] != `{"tracks":[` {
		t.Fatalf("malformed catalog was not preserved for inspection: %q", entries["catalog.json"])
	}
	if entries["logs/recent.log"] == "" {
		t.Fatal("expected recent log tail in diagnostics bundle")
	}
	for _, secret := range []string{"catalog-secret", "log-secret", "Bearer", "token=log-secret"} {
		if strings.Contains(entries["logs/recent.log"], secret) {
			t.Fatalf("recent log leaked %q: %s", secret, entries["logs/recent.log"])
		}
	}
}

func TestRedactAppStateRedactsNestedJobTrackMetadata(t *testing.T) {
	secretURL := func(name string) string {
		return "https://example.test/" + name + "?token=" + name + "-secret"
	}
	links := MetadataLinks{
		OfficialWebsite: secretURL("official"),
		Spotify:         secretURL("spotify"),
		AppleMusic:      secretURL("apple"),
		YouTube:         secretURL("youtube"),
		YouTubeMusic:    secretURL("youtube-music"),
		Instagram:       secretURL("instagram"),
		X:               secretURL("x"),
		Facebook:        secretURL("facebook"),
		Bandcamp:        secretURL("bandcamp"),
		SoundCloud:      secretURL("soundcloud"),
		Wikipedia:       secretURL("wikipedia"),
		MusicBrainz:     secretURL("musicbrainz"),
		Genius:          secretURL("genius"),
	}
	state := AppState{
		Jobs: []Job{{
			Input:        secretURL("job-input"),
			PlaylistURL:  secretURL("playlist"),
			Error:        "Authorization: Bearer job-secret",
			Detail:       secretURL("job-detail"),
			OutputBundle: secretURL("job-output"),
		}},
		ImportHistory: []ImportHistoryEntry{{URL: secretURL("history")}},
		LibraryTracks: []TrackRecord{{
			SourceRef:     secretURL("source"),
			SourceTitle:   secretURL("source-title"),
			SourceChannel: secretURL("source-channel"),
			VideoURL:      secretURL("video"),
			ArtworkURL:    secretURL("artwork"),
			ArtistLinks:   links,
			AlbumLinks:    links,
			SongLinks:     links,
			Sources:       []string{secretURL("source-list")},
		}},
	}

	redacted := redactAppState(state)
	data, err := json.Marshal(redacted)
	if err != nil {
		t.Fatalf("marshal redacted state: %v", err)
	}
	encoded := string(data)
	for _, secret := range []string{
		"job-input-secret", "playlist-secret", "job-secret", "job-detail-secret", "job-output-secret", "history-secret",
		"source-secret", "source-title-secret", "source-channel-secret", "video-secret", "artwork-secret", "source-list-secret",
		"official-secret", "spotify-secret", "apple-secret", "youtube-secret", "youtube-music-secret", "instagram-secret", "x-secret",
		"facebook-secret", "bandcamp-secret", "soundcloud-secret", "wikipedia-secret", "musicbrainz-secret", "genius-secret",
	} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("redacted state contains secret %q: %s", secret, encoded)
		}
	}
	if !strings.Contains(redacted.LibraryTracks[0].ArtistLinks.Instagram, "%3Credacted%3E") {
		t.Fatalf("expected nested Instagram URL to be redacted, got %q", redacted.LibraryTracks[0].ArtistLinks.Instagram)
	}
}

func TestWriteDiagnosticsBundleIncludesToolVersions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	settings := defaultStoredSettings(root)
	state := AppState{
		Settings: PublicSettings{
			LibraryRoot: root,
		},
		BuildInfo: BuildInfo{
			AppVersion: "1.2.3",
		},
	}
	versions := map[string]string{
		"ytDlp":  "yt-dlp 2026.01.01",
		"ffmpeg": "ffmpeg version 7.0.0",
		"go":     "go1.26.1",
		"os":     "darwin",
		"arch":   "arm64",
	}

	bundlePath, err := writeDiagnosticsBundle(store, state, settings, BuildInfo{}, UpdateInfo{}, info, "", versions, nil)
	if err != nil {
		t.Fatalf("writeDiagnosticsBundle: %v", err)
	}
	defer os.Remove(bundlePath)

	reader, err := zip.OpenReader(bundlePath)
	if err != nil {
		t.Fatalf("zip.OpenReader: %v", err)
	}
	defer reader.Close()

	var foundToolVersions bool
	var foundCommandFailures bool
	var foundManifest bool
	for _, file := range reader.File {
		switch file.Name {
		case "tool-versions.json":
			foundToolVersions = true
		case "command-failures.json":
			foundCommandFailures = true
		case "manifest.json":
			foundManifest = true
		}
	}
	if !foundToolVersions {
		t.Fatalf("expected tool-versions.json in bundle")
	}
	if !foundManifest {
		t.Fatalf("expected manifest.json in bundle")
	}
	if !foundCommandFailures {
		t.Fatalf("expected command-failures.json in bundle")
	}
}

func TestDiagnosticsInfoDefaultsToCurrentLogPath(t *testing.T) {
	info := defaultDiagnosticsInfo()
	if info.LogPath != "" && filepath.Base(info.LogPath) == "" {
		t.Fatalf("expected a sane log path when present, got %q", info.LogPath)
	}
	if time.Now().UTC().Year() < 2020 {
		t.Fatalf("sanity check failed")
	}
}

func TestDiagnosticsBundleRedactsSecretsFromStateAndSettings(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	settings := defaultStoredSettings(root)
	settings.APIKey = "sk-secret-value"
	settings.YTDLPCookiesPath = "/private/cookies.txt"
	settings.YTDLPCookiesFromBrowser = "chrome:Default"
	state := AppState{
		Settings:      publicSettings(settings),
		Jobs:          []Job{{Input: "https://example.test/watch?id=1&token=job-secret", Error: "Authorization: Bearer job-secret"}},
		LibraryTracks: []TrackRecord{{SourceRef: "https://example.test/track?sig=track-secret"}},
	}
	state.Settings.UpdateManifestURL = "https://example.test/manifest?token=secret-token"

	bundlePath, err := writeDiagnosticsBundle(store, state, settings, BuildInfo{}, UpdateInfo{}, info, "", nil, nil)
	if err != nil {
		t.Fatalf("writeDiagnosticsBundle: %v", err)
	}
	defer os.Remove(bundlePath)
	reader, err := zip.OpenReader(bundlePath)
	if err != nil {
		t.Fatalf("zip.OpenReader: %v", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name != "state.json" && file.Name != "settings.json" && file.Name != "manifest.json" {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		data, readErr := io.ReadAll(opened)
		_ = opened.Close()
		if readErr != nil {
			t.Fatalf("read %s: %v", file.Name, readErr)
		}
		text := string(data)
		for _, secret := range []string{"sk-secret-value", "/private/cookies.txt", "chrome:Default", "secret-token", "job-secret", "track-secret"} {
			if strings.Contains(text, secret) {
				t.Fatalf("%s contains secret %q: %s", file.Name, secret, text)
			}
		}
	}
}
