package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicToolReadinessIsBoundedAndPathFree(t *testing.T) {
	result := publicToolReadiness(toolReadinessResult{
		ToolName:      "ffmpeg",
		Status:        toolReadinessProbeFailed,
		Version:       "ffmpeg version 7.0\ncompiler details",
		Error:         "permission denied at /Users/example/private-tool",
		Remediation:   "Check the configured path and test it again.\nextra detail",
		CorrelationID: "corr-tool-1",
	})

	if result.Status != string(toolReadinessProbeFailed) {
		t.Fatalf("public status = %q, want %q", result.Status, toolReadinessProbeFailed)
	}
	if result.Version != "ffmpeg version 7.0" || result.Error != "permission denied at /Users/example/private-tool" {
		t.Fatalf("public result did not preserve bounded first-line fields: %#v", result)
	}
	if strings.Contains(result.Remediation, "extra detail") {
		t.Fatalf("public remediation was not bounded: %q", result.Remediation)
	}
	if strings.Contains(result.Error, "tool-secret") {
		t.Fatalf("public readiness leaked a secret: %q", result.Error)
	}
	if result.CorrelationID != "corr-tool-1" {
		t.Fatalf("public correlation ID = %q, want corr-tool-1", result.CorrelationID)
	}
}

func TestInitialToolReadinessExplainsConfiguredAndMissingTools(t *testing.T) {
	app := &App{settings: defaultStoredSettings(t.TempDir())}

	missing := app.initialToolReadinessFor("ffmpeg", filepath.Join(t.TempDir(), "missing-ffmpeg"))
	if missing.Status != toolReadinessMissing {
		t.Fatalf("missing status = %q, want %q", missing.Status, toolReadinessMissing)
	}
	if !strings.Contains(strings.ToLower(missing.Remediation), "settings") {
		t.Fatalf("missing remediation is not actionable: %q", missing.Remediation)
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	unchecked := app.initialToolReadinessFor("helper", executable)
	if unchecked.Status != toolReadinessUnchecked {
		t.Fatalf("configured status = %q, want %q", unchecked.Status, toolReadinessUnchecked)
	}
	if !strings.Contains(strings.ToLower(unchecked.Remediation), "test") {
		t.Fatalf("configured remediation does not direct the user to test: %q", unchecked.Remediation)
	}
}
