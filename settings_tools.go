package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type toolReadinessStatus string

const (
	toolReadinessReady          toolReadinessStatus = "ready"
	toolReadinessMissing        toolReadinessStatus = "missing"
	toolReadinessProbeFailed    toolReadinessStatus = "probe-failed"
	toolReadinessTimedOut       toolReadinessStatus = "timeout"
	toolReadinessInvalidVersion toolReadinessStatus = "invalid-version"
	toolReadinessUnchecked      toolReadinessStatus = "unchecked"
)

type toolReadinessResult struct {
	ToolName      string              `json:"tool"`
	Status        toolReadinessStatus `json:"status"`
	Version       string              `json:"version,omitempty"`
	Error         string              `json:"error,omitempty"`
	Remediation   string              `json:"remediation,omitempty"`
	CorrelationID CorrelationID       `json:"correlationId,omitempty"`
}

const toolProbeTimeout = 20 * time.Second

var (
	ytDLPVersionPattern  = regexp.MustCompile(`(?i)^\s*(?:yt-dlp\s+)?\d{4}\.\d{2}\.\d{2}\b`)
	ffmpegVersionPattern = regexp.MustCompile(`(?i)^\s*ffmpeg\s+version\s+\S+`)
)

func probeToolReadiness(ctx context.Context, path string, args []string, toolName string, timeout time.Duration) toolReadinessResult {
	result := toolReadinessResult{ToolName: toolName, CorrelationID: newCorrelationID()}
	if strings.TrimSpace(path) == "" {
		result.Status = toolReadinessMissing
		result.Error = "executable not found"
		result.Remediation = toolReadinessRemediation(toolName, result.Status)
		return result
	}
	if timeout <= 0 {
		timeout = toolProbeTimeout
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, path, args...)
	stdout, stderr, err := runCommandCaptureWithCorrelation(probeCtx, cmd, result.CorrelationID)
	if probeCtx.Err() == context.DeadlineExceeded {
		result.Status = toolReadinessTimedOut
		result.Error = "probe timed out"
		result.Remediation = toolReadinessRemediation(toolName, result.Status)
		logEvent("tool_readiness_failed", "tool", toolName, "status", result.Status, "correlation_id", result.CorrelationID)
		return result
	}
	if err != nil {
		result.Status = toolReadinessProbeFailed
		result.Error = trimRecordedCommandOutput([]byte(err.Error()))
		if output := trimRecordedCommandOutput(append(stdout, stderr...)); output != "" {
			result.Error = trimRecordedCommandOutput([]byte(result.Error + ": " + output))
		}
		result.Remediation = toolReadinessRemediation(toolName, result.Status)
		logEvent("tool_readiness_failed", "tool", toolName, "status", result.Status, "error", result.Error, "correlation_id", result.CorrelationID)
		return result
	}
	result.Version = trimRecordedCommandOutput(append(stdout, stderr...))
	if result.Version == "" {
		result.Status = toolReadinessProbeFailed
		result.Error = "probe returned no version text"
		result.Remediation = toolReadinessRemediation(toolName, result.Status)
		logEvent("tool_readiness_failed", "tool", toolName, "status", result.Status, "error", result.Error, "correlation_id", result.CorrelationID)
		return result
	}
	if !toolVersionOutputValid(toolName, result.Version) {
		result.Status = toolReadinessInvalidVersion
		result.Error = "probe returned invalid version text"
		result.Remediation = toolReadinessRemediation(toolName, result.Status)
		logEvent("tool_readiness_failed", "tool", toolName, "status", result.Status, "error", result.Error, "correlation_id", result.CorrelationID)
		return result
	}
	result.Status = toolReadinessReady
	result.Remediation = ""
	logEvent("tool_readiness_ready", "tool", toolName, "version", firstLine(result.Version), "correlation_id", result.CorrelationID)
	return result
}

func toolReadinessRemediation(toolName string, status toolReadinessStatus) string {
	label := "the configured tool"
	command := "the tool's version command"
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "yt-dlp", "ytdlp":
		label = "yt-dlp"
		command = "yt-dlp --version"
	case "ffmpeg":
		label = "ffmpeg"
		command = "ffmpeg -version"
	}
	switch status {
	case toolReadinessMissing:
		return fmt.Sprintf("Install %s and ensure it is executable, or set its full executable path in Settings; then run the %s test again.", label, label)
	case toolReadinessProbeFailed:
		return fmt.Sprintf("Verify the configured %s path and run `%s` manually; reinstall or update %s if that command fails, then test it again.", label, command, label)
	case toolReadinessInvalidVersion:
		return fmt.Sprintf("The configured %s did not return a recognizable version. Verify the executable path and run `%s` manually, then reinstall or update %s if needed.", label, command, label)
	case toolReadinessTimedOut:
		return fmt.Sprintf("The %s probe timed out. Check that the executable is local and responsive, choose another path in Settings if needed, and test it again.", label)
	case toolReadinessUnchecked:
		return fmt.Sprintf("Run the %s test to verify the configured executable before starting an import.", label)
	default:
		return ""
	}
}

func toolVersionOutputValid(toolName, output string) bool {
	line := firstLine(output)
	if line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "yt-dlp", "ytdlp":
		return ytDLPVersionPattern.MatchString(line)
	case "ffmpeg":
		return ffmpegVersionPattern.MatchString(line)
	default:
		return true
	}
}

func (a *App) initialToolReadiness() map[string]toolReadinessResult {
	return map[string]toolReadinessResult{
		"ytDlp":  a.initialToolReadinessFor("yt-dlp", a.settings.YTDLPPath),
		"ffmpeg": a.initialToolReadinessFor("ffmpeg", a.settings.FFmpegPath),
	}
}

func (a *App) initialToolReadinessFor(toolName, configured string) toolReadinessResult {
	result := toolReadinessResult{ToolName: toolName, CorrelationID: newCorrelationID()}
	if strings.TrimSpace(a.toolPath(configured, toolName)) == "" {
		result.Status = toolReadinessMissing
		result.Error = "executable not found"
		result.Remediation = toolReadinessRemediation(toolName, result.Status)
		return result
	}
	result.Status = toolReadinessUnchecked
	result.Remediation = toolReadinessRemediation(toolName, result.Status)
	return result
}

func publicToolReadiness(result toolReadinessResult) ToolReadiness {
	return ToolReadiness{
		ToolName:      result.ToolName,
		Status:        string(result.Status),
		Version:       firstLine(result.Version),
		Error:         firstLine(result.Error),
		Remediation:   firstLine(result.Remediation),
		CorrelationID: string(result.CorrelationID),
	}
}

func (a *App) toolReadinessLocked() map[string]ToolReadiness {
	if a.toolReadinessCache == nil {
		a.toolReadinessCache = a.initialToolReadiness()
	}
	readiness := make(map[string]ToolReadiness, len(a.toolReadinessCache))
	for key, result := range a.toolReadinessCache {
		readiness[key] = publicToolReadiness(result)
	}
	return readiness
}

func (a *App) rememberToolReadiness(toolKey string, result toolReadinessResult) {
	a.mu.Lock()
	if a.toolReadinessCache == nil {
		a.toolReadinessCache = a.initialToolReadiness()
	}
	a.toolReadinessCache[toolKey] = result
	state := a.snapshotLocked()
	a.mu.Unlock()
	a.emitStateLocked(state)
}

func firstLine(value string) string {
	return strings.SplitN(strings.TrimSpace(value), "\n", 2)[0]
}

func (a *App) toolReadiness(toolName, configured string, args []string) toolReadinessResult {
	path := a.toolPath(configured, toolName)
	return probeToolReadiness(a.appContext(), path, args, toolName, toolProbeTimeout)
}

func (a *App) TestYTDLP() (string, error) {
	a.mu.Lock()
	configured := a.settings.YTDLPPath
	a.mu.Unlock()
	result := a.toolReadiness("yt-dlp", configured, []string{"--version"})
	a.rememberToolReadiness("ytDlp", result)
	if result.Status == toolReadinessMissing {
		logEvent("tool_test_failed", "tool", "yt-dlp", "reason", "unavailable")
		return "", fmt.Errorf("yt-dlp is not available: %s", result.Remediation)
	}
	if result.Status != toolReadinessReady {
		logEvent("tool_test_failed", "tool", "yt-dlp", "status", result.Status, "error", result.Error, "correlation_id", result.CorrelationID)
		return "", fmt.Errorf("yt-dlp test failed: %s. %s", result.Error, result.Remediation)
	}
	message := fmt.Sprintf("yt-dlp %s", firstLine(result.Version))
	logEvent("tool_test_passed", "tool", "yt-dlp", "result", message, "correlation_id", result.CorrelationID)
	return message, nil
}

func (a *App) TestFFmpeg() (string, error) {
	a.mu.Lock()
	configured := a.settings.FFmpegPath
	a.mu.Unlock()
	result := a.toolReadiness("ffmpeg", configured, []string{"-version"})
	a.rememberToolReadiness("ffmpeg", result)
	if result.Status == toolReadinessMissing {
		logEvent("tool_test_failed", "tool", "ffmpeg", "reason", "unavailable")
		return "", fmt.Errorf("ffmpeg is not available: %s", result.Remediation)
	}
	if result.Status != toolReadinessReady {
		logEvent("tool_test_failed", "tool", "ffmpeg", "status", result.Status, "error", result.Error, "correlation_id", result.CorrelationID)
		return "", fmt.Errorf("ffmpeg test failed: %s. %s", result.Error, result.Remediation)
	}
	message := firstLine(result.Version)
	if message == "" {
		message = "ffmpeg reachable"
	}
	logEvent("tool_test_passed", "tool", "ffmpeg", "result", message, "correlation_id", result.CorrelationID)
	return message, nil
}

func (a *App) TestAI() (string, error) {
	if a.ai == nil || !a.ai.available() {
		logEvent("tool_test_failed", "tool", "ai", "reason", "unavailable")
		return "", errors.New("AI is unavailable")
	}
	ctx, cancel := context.WithTimeout(a.appContext(), 30*time.Second)
	defer cancel()
	result, err := a.ai.testConnection(ctx)
	if err != nil {
		logEvent("tool_test_failed", "tool", "ai", "error", err.Error())
		return "", err
	}
	logEvent("tool_test_passed", "tool", "ai", "result", result)
	return result, nil
}

func (a *App) ResetSettings() (AppState, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	reset := defaultStoredSettings(a.settings.LibraryRoot)
	a.settings = reset
	_ = a.store.saveSettings(a.settings)
	_ = saveRootPreference(a.settings.LibraryRoot)
	a.ai = newAIClient(a.settings)
	a.aiStatus = aiStatusForSettings(a.settings, a.promptLibrary)
	a.updateInfo = defaultUpdateInfo(a.settings, a.buildInfo)
	state := a.snapshotLocked()
	a.emitStateLocked(state)
	logEvent("settings_reset", "library_root", a.settings.LibraryRoot)
	return state, nil
}

func (a *App) appContext() context.Context {
	if a.appCtx != nil {
		return a.appCtx
	}
	return context.Background()
}
