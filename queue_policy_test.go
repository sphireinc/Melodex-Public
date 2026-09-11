package main

import "testing"

func TestVideoDownloadModeMigratesLegacyBoolean(t *testing.T) {
	legacyImport := migrateStoredSettings(storedSettings{PublicSettings: PublicSettings{DownloadMusicVideo: true}}, "/tmp/music", 2)
	if legacyImport.VideoDownloadMode != videoDownloadModeDuringImport || !legacyImport.DownloadMusicVideo {
		t.Fatalf("legacy enabled video setting did not migrate: %+v", legacyImport)
	}
	legacyOnDemand := migrateStoredSettings(storedSettings{PublicSettings: PublicSettings{DownloadMusicVideo: false}}, "/tmp/music", 2)
	if legacyOnDemand.VideoDownloadMode != videoDownloadModeOnDemand || legacyOnDemand.DownloadMusicVideo {
		t.Fatalf("legacy disabled video setting did not migrate: %+v", legacyOnDemand)
	}
	if got := normalizeVideoDownloadMode("unexpected"); got != videoDownloadModeOnDemand {
		t.Fatalf("unexpected mode normalized to %q", got)
	}
}

func TestValidateSettingsInputBoundsConcurrencyAndVideoMode(t *testing.T) {
	base := SettingsInput{VideoDownloadMode: videoDownloadModeOnDemand}
	if err := validateSettingsInput(base); err != nil {
		t.Fatalf("valid settings rejected: %v", err)
	}
	base.MaxConcurrentDownloads = maxConfiguredConcurrency + 1
	if err := validateSettingsInput(base); err == nil {
		t.Fatal("expected excessive concurrency to be rejected")
	}
	base.MaxConcurrentDownloads = -1
	if err := validateSettingsInput(base); err == nil {
		t.Fatal("expected negative concurrency to be rejected")
	}
	base.MaxConcurrentDownloads = 1
	base.VideoDownloadMode = "invalid"
	if err := validateSettingsInput(base); err == nil {
		t.Fatal("expected invalid video mode to be rejected")
	}
}
