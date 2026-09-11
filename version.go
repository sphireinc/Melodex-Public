package main

import (
	"fmt"
	"runtime"
	"strings"
)

var (
	AppVersion     = "1.0.0"
	BuildNumber    = "dev"
	GitCommit      = "dev"
	BuildTime      = "unknown"
	ReleaseChannel = "dev"
)

const (
	settingsSchemaVersion      = 3
	catalogSchemaVersion       = 1
	libraryCacheSchemaVersion  = 1
	playlistSchemaVersion      = 1
	importHistorySchemaVersion = 1
	trackMetadataSchemaVersion = 1
)

func currentBuildInfo() BuildInfo {
	return BuildInfo{
		AppVersion:                 cleanVersion(AppVersion),
		BuildNumber:                cleanMetadata(BuildNumber, "dev"),
		GitCommit:                  cleanMetadata(GitCommit, "dev"),
		BuildTime:                  cleanMetadata(BuildTime, "unknown"),
		ReleaseChannel:             cleanMetadata(ReleaseChannel, "dev"),
		GoVersion:                  runtime.Version(),
		SettingsSchemaVersion:      settingsSchemaVersion,
		CatalogSchemaVersion:       catalogSchemaVersion,
		LibraryCacheSchemaVersion:  libraryCacheSchemaVersion,
		PlaylistSchemaVersion:      playlistSchemaVersion,
		ImportHistorySchemaVersion: importHistorySchemaVersion,
		TrackMetadataSchemaVersion: trackMetadataSchemaVersion,
	}
}

func buildSummary() string {
	info := currentBuildInfo()
	return fmt.Sprintf("%s build=%s commit=%s channel=%s built=%s", info.AppVersion, info.BuildNumber, info.GitCommit, info.ReleaseChannel, info.BuildTime)
}

func cleanVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "1.0.0"
	}
	return value
}

func cleanMetadata(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
