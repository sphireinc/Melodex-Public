package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Store struct {
	root              string
	appDataDir        string
	settingsPath      string
	catalogPath       string
	libraryCachePath  string
	pipelineCachePath string
	playlistsPath     string
	importHistoryPath string
}

const (
	videoDownloadModeOff          = "off"
	videoDownloadModeOnDemand     = "on-demand"
	videoDownloadModeDuringImport = "during-import"
	maxConfiguredConcurrency      = 8
)

type libraryCacheFile struct {
	Version   int           `json:"version"`
	Catalog   catalogFile   `json:"catalog"`
	Stats     LibraryStats  `json:"stats"`
	Health    LibraryHealth `json:"health"`
	Genres    []FacetBucket `json:"genres,omitempty"`
	Years     []FacetBucket `json:"years,omitempty"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

func defaultPublicSettings(root string) PublicSettings {
	return PublicSettings{
		LibraryRoot:                     root,
		AIBaseURL:                       "https://api.openai.com/v1",
		AIModel:                         "gpt-4.1-mini",
		Provider:                        "openai-compatible",
		UpdateManifestURL:               defaultUpdateManifestURL(storedSettings{}),
		VideoDownloadMode:               videoDownloadModeOnDemand,
		DownloadMusicVideo:              false,
		KeepOriginalAudio:               false,
		MaxConcurrentDownloads:          2,
		MaxConcurrentVideoDownloads:     1,
		MaxConcurrentEnrichmentRequests: 2,
		MaxConcurrentLyricsRequests:     2,
		ThrottleOnYTDLPBotErrors:        true,
	}
}

func defaultStoredSettings(root string) storedSettings {
	return storedSettings{
		PublicSettings: defaultPublicSettings(root),
		APIKey:         "",
		SchemaVersion:  settingsSchemaVersion,
	}
}

func normalizeStoredSettingsDefaults(settings *storedSettings, root string, originalSchemaVersion int) {
	if settings == nil {
		return
	}
	if strings.TrimSpace(settings.LibraryRoot) == "" {
		settings.LibraryRoot = root
	}
	if strings.TrimSpace(settings.AIBaseURL) == "" {
		settings.AIBaseURL = "https://api.openai.com/v1"
	}
	if strings.TrimSpace(settings.AIModel) == "" {
		settings.AIModel = "gpt-4.1-mini"
	}
	if strings.TrimSpace(settings.Provider) == "" {
		settings.Provider = "openai-compatible"
	}
	if settings.UpdateManifestURL == "" {
		settings.UpdateManifestURL = defaultUpdateManifestURL(*settings)
	}
	if strings.TrimSpace(settings.VideoDownloadMode) == "" {
		settings.VideoDownloadMode = videoDownloadModeOnDemand
		if settings.DownloadMusicVideo {
			settings.VideoDownloadMode = videoDownloadModeDuringImport
		}
	}
	settings.VideoDownloadMode = normalizeVideoDownloadMode(settings.VideoDownloadMode)
	settings.DownloadMusicVideo = settings.VideoDownloadMode == videoDownloadModeDuringImport
	if settings.MaxConcurrentDownloads <= 0 {
		settings.MaxConcurrentDownloads = 2
	}
	if settings.MaxConcurrentDownloads > maxConfiguredConcurrency {
		settings.MaxConcurrentDownloads = maxConfiguredConcurrency
	}
	if settings.MaxConcurrentVideoDownloads <= 0 {
		settings.MaxConcurrentVideoDownloads = 1
	}
	if settings.MaxConcurrentVideoDownloads > maxConfiguredConcurrency {
		settings.MaxConcurrentVideoDownloads = maxConfiguredConcurrency
	}
	if settings.MaxConcurrentEnrichmentRequests <= 0 {
		settings.MaxConcurrentEnrichmentRequests = 2
	}
	if settings.MaxConcurrentEnrichmentRequests > maxConfiguredConcurrency {
		settings.MaxConcurrentEnrichmentRequests = maxConfiguredConcurrency
	}
	if settings.MaxConcurrentLyricsRequests <= 0 {
		settings.MaxConcurrentLyricsRequests = 2
	}
	if settings.MaxConcurrentLyricsRequests > maxConfiguredConcurrency {
		settings.MaxConcurrentLyricsRequests = maxConfiguredConcurrency
	}
	if originalSchemaVersion < 3 && !settings.KeepOriginalAudio {
		settings.KeepOriginalAudio = false
	}
	if originalSchemaVersion < 2 && !settings.ThrottleOnYTDLPBotErrors {
		settings.ThrottleOnYTDLPBotErrors = true
	}
}

func normalizeVideoDownloadMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case videoDownloadModeOff:
		return videoDownloadModeOff
	case videoDownloadModeDuringImport:
		return videoDownloadModeDuringImport
	case videoDownloadModeOnDemand:
		return videoDownloadModeOnDemand
	default:
		return videoDownloadModeOnDemand
	}
}

func newStore(root string) (*Store, RootInfo, error) {
	info, err := ensureLibraryLayout(root)
	if err != nil {
		return nil, RootInfo{}, err
	}
	return &Store{
		root:              info.LibraryRoot,
		appDataDir:        info.AppDataDir,
		settingsPath:      filepath.Join(info.LibraryRoot, "settings.json"),
		catalogPath:       filepath.Join(info.AppDataDir, "catalog.json"),
		libraryCachePath:  filepath.Join(info.CacheDir, "library-index.json"),
		pipelineCachePath: filepath.Join(info.CacheDir, "pipeline-cache.json"),
		playlistsPath:     filepath.Join(info.AppDataDir, "playlists.json"),
		importHistoryPath: filepath.Join(info.AppDataDir, "imports.json"),
	}, info, nil
}

func (s *Store) loadSettings() (storedSettings, error) {
	data, err := os.ReadFile(s.settingsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultStoredSettings(s.root), nil
		}
		return storedSettings{}, err
	}
	var settings storedSettings
	if err := decodeAuthoritativeJSON(data, &settings, "settings"); err != nil {
		return storedSettings{}, err
	}
	if err := validatePersistedSchemaVersion("settings", settings.SchemaVersion, settingsSchemaVersion); err != nil {
		return storedSettings{}, err
	}
	return migrateStoredSettings(settings, s.root, settings.SchemaVersion), nil
}

func (s *Store) saveSettings(settings storedSettings) error {
	if err := ensureDir(s.root); err != nil {
		return err
	}
	if settings.SchemaVersion == 0 {
		settings.SchemaVersion = settingsSchemaVersion
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicFile(s.settingsPath, data, 0o600)
}

func (s *Store) loadCatalog() (catalogFile, error) {
	data, err := os.ReadFile(s.catalogPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return catalogFile{Version: catalogSchemaVersion, Tracks: []TrackRecord{}, Jobs: []Job{}}, nil
		}
		return catalogFile{}, err
	}
	var catalog catalogFile
	if err := decodeAuthoritativeJSON(data, &catalog, "catalog"); err != nil {
		return catalogFile{}, err
	}
	if err := validatePersistedSchemaVersion("catalog", catalog.Version, catalogSchemaVersion); err != nil {
		return catalogFile{}, err
	}
	return migrateCatalogFile(catalog), nil
}

func (s *Store) saveCatalog(c catalogFile) error {
	if err := ensureDir(s.appDataDir); err != nil {
		return err
	}
	if c.Version == 0 {
		c.Version = catalogSchemaVersion
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicFile(s.catalogPath, data, 0o600)
}

func (s *Store) loadLibraryCache() (catalogFile, error) {
	data, err := os.ReadFile(s.libraryCachePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return catalogFile{Version: libraryCacheSchemaVersion, Tracks: []TrackRecord{}, Jobs: []Job{}}, nil
		}
		return catalogFile{}, err
	}
	isCache, err := hasJSONField(data, "catalog")
	if err != nil {
		return catalogFile{}, err
	}
	if isCache {
		var cached libraryCacheFile
		if err := decodeCacheJSON(data, &cached, "library cache"); err != nil {
			return catalogFile{}, err
		}
		if err := validatePersistedSchemaVersion("library cache", cached.Version, libraryCacheSchemaVersion); err != nil {
			// The index is derived data. Discarding an incompatible index and
			// rebuilding it is safer than blocking the library on cache state.
			return catalogFile{Version: catalogSchemaVersion, Tracks: []TrackRecord{}, Jobs: []Job{}}, nil
		}
		if err := validatePersistedSchemaVersion("library cache catalog", cached.Catalog.Version, catalogSchemaVersion); err != nil {
			return catalogFile{Version: catalogSchemaVersion, Tracks: []TrackRecord{}, Jobs: []Job{}}, nil
		}
		return migrateLibraryCacheFile(cached).Catalog, nil
	}
	var catalog catalogFile
	if err := decodeAuthoritativeJSON(data, &catalog, "catalog"); err != nil {
		return catalogFile{}, err
	}
	if err := validatePersistedSchemaVersion("catalog", catalog.Version, catalogSchemaVersion); err != nil {
		return catalogFile{}, err
	}
	return migrateCatalogFile(catalog), nil
}

func (s *Store) saveLibraryCache(c catalogFile) error {
	if err := ensureDir(s.appDataDir); err != nil {
		return err
	}
	if err := ensureDir(filepath.Dir(s.libraryCachePath)); err != nil {
		return err
	}
	cache := libraryCacheFile{
		Version:   libraryCacheSchemaVersion,
		Catalog:   c,
		Stats:     summarizeCatalogStats(c),
		Health:    summarizeCatalogHealth(c),
		Genres:    summarizeCatalogGenreCounts(c),
		Years:     summarizeCatalogYearCounts(c),
		UpdatedAt: time.Now().UTC(),
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicFile(s.libraryCachePath, data, 0o600)
}

func summarizeCatalogStats(c catalogFile) LibraryStats {
	stats := LibraryStats{}
	artistSet := map[string]struct{}{}
	albumSet := map[string]struct{}{}
	for _, track := range c.Tracks {
		stats.TrackCount++
		artistSet[strings.ToLower(track.Artist)] = struct{}{}
		albumSet[strings.ToLower(track.Artist+"::"+track.Album)] = struct{}{}
	}
	stats.ArtistCount = len(artistSet)
	stats.AlbumCount = len(albumSet)
	stats.JobCount = len(c.Jobs)
	for _, job := range c.Jobs {
		switch job.Status {
		case "queued", "running":
			stats.PendingJobs++
		case "failed":
			stats.FailedJobs++
		}
	}
	return stats
}

func summarizeCatalogHealth(c catalogFile) LibraryHealth {
	health := LibraryHealth{}
	for _, track := range c.Tracks {
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

func summarizeCatalogGenreCounts(c catalogFile) []FacetBucket {
	counts := map[string]FacetBucket{}
	for _, track := range c.Tracks {
		genre := strings.TrimSpace(track.Genre)
		if genre == "" {
			continue
		}
		key := strings.ToLower(genre)
		bucket := counts[key]
		if bucket.Value == "" {
			bucket.Value = genre
		}
		bucket.Count++
		counts[key] = bucket
	}
	return countsToSortedSlice(counts)
}

func summarizeCatalogYearCounts(c catalogFile) []FacetBucket {
	counts := map[string]FacetBucket{}
	for _, track := range c.Tracks {
		year := strings.TrimSpace(track.Year)
		if year == "" {
			continue
		}
		bucket := counts[year]
		if bucket.Value == "" {
			bucket.Value = year
		}
		bucket.Count++
		counts[year] = bucket
	}
	return countsToSortedSlice(counts)
}

func countsToSortedSlice(counts map[string]FacetBucket) []FacetBucket {
	items := make([]FacetBucket, 0, len(counts))
	for _, bucket := range counts {
		items = append(items, bucket)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Value < items[j].Value
		}
		return items[i].Count > items[j].Count
	})
	return items
}

func (s *Store) loadPlaylists() (playlistFile, error) {
	data, err := os.ReadFile(s.playlistsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return playlistFile{Version: playlistSchemaVersion, Playlists: []Playlist{}}, nil
		}
		return playlistFile{}, err
	}
	var playlists playlistFile
	if err := decodeAuthoritativeJSON(data, &playlists, "playlists"); err != nil {
		return playlistFile{}, err
	}
	if err := validatePersistedSchemaVersion("playlist", playlists.Version, playlistSchemaVersion); err != nil {
		return playlistFile{}, err
	}
	return migratePlaylistFile(playlists), nil
}

func (s *Store) savePlaylists(p playlistFile) error {
	if err := ensureDir(s.appDataDir); err != nil {
		return err
	}
	if p.Version == 0 {
		p.Version = playlistSchemaVersion
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicFile(s.playlistsPath, data, 0o600)
}

func (s *Store) loadImportHistory() (importHistoryFile, error) {
	data, err := os.ReadFile(s.importHistoryPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return importHistoryFile{Version: importHistorySchemaVersion, Entries: []ImportHistoryEntry{}}, nil
		}
		return importHistoryFile{}, err
	}
	var history importHistoryFile
	if err := decodeAuthoritativeJSON(data, &history, "import history"); err != nil {
		return importHistoryFile{}, err
	}
	if err := validatePersistedSchemaVersion("import history", history.Version, importHistorySchemaVersion); err != nil {
		return importHistoryFile{}, err
	}
	return migrateImportHistoryFile(history), nil
}

func (s *Store) saveImportHistory(history importHistoryFile) error {
	if err := ensureDir(s.appDataDir); err != nil {
		return err
	}
	if history.Version == 0 {
		history.Version = importHistorySchemaVersion
	}
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicFile(s.importHistoryPath, data, 0o600)
}
