package main

import "melodex/internal/songstore"

func migrateStoredSettings(settings storedSettings, root string, originalSchemaVersion int) storedSettings {
	if settings.SchemaVersion == 0 {
		settings.SchemaVersion = settingsSchemaVersion
	}
	normalizeStoredSettingsDefaults(&settings, root, originalSchemaVersion)
	settings.SchemaVersion = settingsSchemaVersion
	return settings
}

func migrateCatalogFile(c catalogFile) catalogFile {
	if c.Version == 0 {
		c.Version = catalogSchemaVersion
	}
	if c.Tracks == nil {
		c.Tracks = []TrackRecord{}
	}
	if c.Jobs == nil {
		c.Jobs = []Job{}
	}
	return c
}

func migrateLibraryCacheFile(c libraryCacheFile) libraryCacheFile {
	if c.Version == 0 {
		c.Version = libraryCacheSchemaVersion
	}
	c.Catalog = migrateCatalogFile(c.Catalog)
	return c
}

func migratePlaylistFile(p playlistFile) playlistFile {
	if p.Version == 0 {
		p.Version = playlistSchemaVersion
	}
	if p.Playlists == nil {
		p.Playlists = []Playlist{}
	}
	for i := range p.Playlists {
		if p.Playlists[i].TrackIDs == nil {
			p.Playlists[i].TrackIDs = []string{}
		}
	}
	return p
}

func migrateImportHistoryFile(h importHistoryFile) importHistoryFile {
	if h.Version == 0 {
		h.Version = importHistorySchemaVersion
	}
	if h.Entries == nil {
		h.Entries = []ImportHistoryEntry{}
	}
	return h
}

func migrateTrackMetadata(metadata songstore.SongMetadata) songstore.SongMetadata {
	if metadata.Version == 0 {
		metadata.Version = trackMetadataSchemaVersion
	}
	metadata, _ = songstore.EnsureAIStatus(metadata)
	return metadata
}
