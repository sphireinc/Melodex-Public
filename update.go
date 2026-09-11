package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type updateManifest struct {
	Version        string                        `json:"version"`
	ReleaseDate    time.Time                     `json:"release_date,omitempty"`
	ReleaseNotes   string                        `json:"release_notes,omitempty"`
	MinimumVersion string                        `json:"minimum_version,omitempty"`
	Mandatory      bool                          `json:"mandatory,omitempty"`
	Platforms      map[string]updateManifestPlat `json:"platforms,omitempty"`
}

type updateManifestPlat struct {
	URL      string `json:"url"`
	Size     int64  `json:"size,omitempty"`
	Checksum string `json:"checksum,omitempty"`
}

func defaultUpdateManifestURL(settings storedSettings) string {
	if value := strings.TrimSpace(settings.UpdateManifestURL); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("MELODEX_UPDATE_MANIFEST_URL"))
}

func defaultUpdateInfo(settings storedSettings, buildInfo BuildInfo) UpdateInfo {
	manifestURL := defaultUpdateManifestURL(settings)
	return UpdateInfo{
		ManifestURL:    manifestURL,
		CurrentVersion: buildInfo.AppVersion,
		Platform:       currentUpdatePlatformKey(),
		Status:         updateStatusForURL(manifestURL),
		CheckedAt:      time.Time{},
	}
}

func updateStatusForURL(manifestURL string) string {
	if strings.TrimSpace(manifestURL) == "" {
		return "Update manifest not configured"
	}
	return "Update check not run"
}

func currentUpdatePlatformKey() string {
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return "macos-arm64"
		}
		return "macos-amd64"
	case "windows":
		return "windows-amd64"
	case "linux":
		if runtime.GOARCH == "arm64" {
			return "linux-arm64"
		}
		return "linux-amd64"
	default:
		return runtime.GOOS + "-" + runtime.GOARCH
	}
}

func compareSemanticVersions(left, right string) (int, error) {
	lp, lpre, err := parseSemanticVersion(left)
	if err != nil {
		return 0, err
	}
	rp, rpre, err := parseSemanticVersion(right)
	if err != nil {
		return 0, err
	}
	for i := 0; i < 3; i++ {
		if lp[i] > rp[i] {
			return 1, nil
		}
		if lp[i] < rp[i] {
			return -1, nil
		}
	}
	switch {
	case lpre == "" && rpre == "":
		return 0, nil
	case lpre == "" && rpre != "":
		return 1, nil
	case lpre != "" && rpre == "":
		return -1, nil
	default:
		switch {
		case lpre > rpre:
			return 1, nil
		case lpre < rpre:
			return -1, nil
		default:
			return 0, nil
		}
	}
}

func parseSemanticVersion(value string) ([3]int, string, error) {
	var parts [3]int
	value = strings.TrimSpace(strings.TrimPrefix(value, "v"))
	if value == "" {
		return parts, "", errors.New("empty version")
	}
	main := value
	preRelease := ""
	if dash := strings.Index(main, "-"); dash >= 0 {
		preRelease = strings.TrimSpace(main[dash+1:])
		main = main[:dash]
	}
	segments := strings.Split(main, ".")
	if len(segments) < 3 {
		return parts, "", fmt.Errorf("version %q must have major.minor.patch", value)
	}
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(strings.TrimSpace(segments[i]))
		if err != nil {
			return parts, "", fmt.Errorf("version %q has invalid numeric segment: %w", value, err)
		}
		parts[i] = n
	}
	return parts, preRelease, nil
}

func checkForUpdates(settings storedSettings, buildInfo BuildInfo) UpdateInfo {
	info := defaultUpdateInfo(settings, buildInfo)
	manifestURL := strings.TrimSpace(info.ManifestURL)
	if manifestURL == "" {
		logEvent("update_check_skipped", "reason", "missing_manifest_url")
		return info
	}
	logEvent("update_check_started", "manifest_url", manifestURL, "current_version", buildInfo.AppVersion, "platform", info.Platform)

	req, err := http.NewRequest(http.MethodGet, manifestURL, nil)
	if err != nil {
		info.Status = "Update check failed"
		info.Error = err.Error()
		info.CheckedAt = time.Now().UTC()
		logEvent("update_check_failed", "manifest_url", manifestURL, "error", err.Error())
		return info
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		info.Status = "Update check failed"
		info.Error = err.Error()
		info.CheckedAt = time.Now().UTC()
		logEvent("update_check_failed", "manifest_url", manifestURL, "error", err.Error())
		return info
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		info.Status = "Update check failed"
		info.Error = fmt.Sprintf("manifest request returned %s", resp.Status)
		info.CheckedAt = time.Now().UTC()
		logEvent("update_check_failed", "manifest_url", manifestURL, "status", resp.Status)
		return info
	}

	var manifest updateManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		info.Status = "Update check failed"
		info.Error = err.Error()
		info.CheckedAt = time.Now().UTC()
		logEvent("update_check_failed", "manifest_url", manifestURL, "error", err.Error())
		return info
	}

	info.CheckedAt = time.Now().UTC()
	info.LatestVersion = strings.TrimSpace(manifest.Version)
	info.ReleaseDate = manifest.ReleaseDate
	info.ReleaseNotes = strings.TrimSpace(manifest.ReleaseNotes)
	info.MinimumVersion = strings.TrimSpace(manifest.MinimumVersion)
	info.Mandatory = manifest.Mandatory
	info.Platform = currentUpdatePlatformKey()

	platform, ok := manifest.Platforms[info.Platform]
	if !ok {
		info.Status = fmt.Sprintf("No update artifact for %s", info.Platform)
		info.Error = ""
		logEvent("update_check_completed", "manifest_url", manifestURL, "status", info.Status, "available", false)
		return info
	}
	info.DownloadURL = strings.TrimSpace(platform.URL)

	cmp, err := compareSemanticVersions(buildInfo.AppVersion, info.LatestVersion)
	if err != nil {
		info.Status = "Update check failed"
		info.Error = err.Error()
		logEvent("update_check_failed", "manifest_url", manifestURL, "error", err.Error())
		return info
	}
	if info.MinimumVersion != "" {
		minCmp, err := compareSemanticVersions(buildInfo.AppVersion, info.MinimumVersion)
		if err != nil {
			info.Status = "Update check failed"
			info.Error = err.Error()
			logEvent("update_check_failed", "manifest_url", manifestURL, "error", err.Error())
			return info
		}
		if minCmp < 0 {
			info.Available = true
			info.Status = "Update required"
			logEvent("update_check_completed", "manifest_url", manifestURL, "status", info.Status, "available", true, "mandatory", true)
			return info
		}
	}
	if cmp < 0 {
		info.Available = true
		info.Status = "Update available"
		logEvent("update_check_completed", "manifest_url", manifestURL, "status", info.Status, "available", true, "mandatory", info.Mandatory)
		return info
	}
	info.Available = false
	info.Status = "Up to date"
	logEvent("update_check_completed", "manifest_url", manifestURL, "status", info.Status, "available", false)
	return info
}

func (a *App) CheckForUpdates() (AppState, error) {
	a.mu.Lock()
	settings := a.settings
	buildInfo := a.buildInfo
	a.mu.Unlock()

	updateInfo := checkForUpdates(settings, buildInfo)
	logEvent("update_check_requested", "manifest_url", updateInfo.ManifestURL, "current_version", buildInfo.AppVersion)

	a.mu.Lock()
	a.updateInfo = updateInfo
	state := a.snapshotLocked()
	a.mu.Unlock()
	a.emitStateLocked(state)
	return state, nil
}

func (a *App) OpenURL(url string) error {
	if a.wailsApp == nil {
		return errors.New("app not started")
	}
	normalized, err := validateHTTPURL(url)
	if err != nil {
		return err
	}
	logEvent("open_url_requested", "url", normalized)
	return a.wailsApp.Browser.OpenURL(normalized)
}
