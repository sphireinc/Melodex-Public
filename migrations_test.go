package main

import (
	"testing"

	"melodex/internal/songstore"
)

func TestMigrationHelpersNormalizeSchemaVersions(t *testing.T) {
	settings := migrateStoredSettings(storedSettings{}, "/tmp/root", 0)
	if settings.SchemaVersion != settingsSchemaVersion {
		t.Fatalf("expected settings schema version %d, got %d", settingsSchemaVersion, settings.SchemaVersion)
	}

	catalog := migrateCatalogFile(catalogFile{})
	if catalog.Version != catalogSchemaVersion {
		t.Fatalf("expected catalog schema version %d, got %d", catalogSchemaVersion, catalog.Version)
	}

	cache := migrateLibraryCacheFile(libraryCacheFile{})
	if cache.Version != libraryCacheSchemaVersion {
		t.Fatalf("expected cache schema version %d, got %d", libraryCacheSchemaVersion, cache.Version)
	}

	playlists := migratePlaylistFile(playlistFile{Playlists: nil})
	if playlists.Version != playlistSchemaVersion {
		t.Fatalf("expected playlist schema version %d, got %d", playlistSchemaVersion, playlists.Version)
	}

	history := migrateImportHistoryFile(importHistoryFile{})
	if history.Version != importHistorySchemaVersion {
		t.Fatalf("expected import history schema version %d, got %d", importHistorySchemaVersion, history.Version)
	}

	metadata := migrateTrackMetadata(songstore.SongMetadata{})
	if metadata.Version != trackMetadataSchemaVersion {
		t.Fatalf("expected track metadata schema version %d, got %d", trackMetadataSchemaVersion, metadata.Version)
	}
}
