package main

import (
	"time"

	"melodex/internal/songstore"
)

type SongAIContext = songstore.SongAIContext

type PublicSettings struct {
	LibraryRoot                     string `json:"libraryRoot"`
	AIBaseURL                       string `json:"aiBaseUrl"`
	AIModel                         string `json:"aiModel"`
	Provider                        string `json:"provider"`
	APIKeyConfigured                bool   `json:"apiKeyConfigured"`
	UpdateManifestURL               string `json:"updateManifestUrl,omitempty"`
	YTDLPPath                       string `json:"ytDlpPath"`
	FFmpegPath                      string `json:"ffmpegPath"`
	YTDLPCookiesPath                string `json:"ytDlpCookiesPath"`
	YTDLPCookiesFromBrowser         string `json:"ytDlpCookiesFromBrowser"`
	VideoDownloadMode               string `json:"videoDownloadMode"`
	DownloadMusicVideo              bool   `json:"downloadMusicVideo"`
	KeepOriginalAudio               bool   `json:"keepOriginalAudio"`
	MaxConcurrentDownloads          int    `json:"maxConcurrentDownloads"`
	MaxConcurrentVideoDownloads     int    `json:"maxConcurrentVideoDownloads"`
	MaxConcurrentEnrichmentRequests int    `json:"maxConcurrentEnrichmentRequests"`
	MaxConcurrentLyricsRequests     int    `json:"maxConcurrentLyricsRequests"`
	ThrottleOnYTDLPBotErrors        bool   `json:"throttleOnYtdlpBotErrors"`
}

type SettingsInput struct {
	LibraryRoot                     string `json:"libraryRoot"`
	AIBaseURL                       string `json:"aiBaseUrl"`
	AIModel                         string `json:"aiModel"`
	Provider                        string `json:"provider"`
	APIKey                          string `json:"apiKey"`
	UpdateManifestURL               string `json:"updateManifestUrl"`
	YTDLPPath                       string `json:"ytDlpPath"`
	FFmpegPath                      string `json:"ffmpegPath"`
	YTDLPCookiesPath                string `json:"ytDlpCookiesPath"`
	YTDLPCookiesFromBrowser         string `json:"ytDlpCookiesFromBrowser"`
	VideoDownloadMode               string `json:"videoDownloadMode"`
	DownloadMusicVideo              bool   `json:"downloadMusicVideo"`
	KeepOriginalAudio               bool   `json:"keepOriginalAudio"`
	MaxConcurrentDownloads          int    `json:"maxConcurrentDownloads"`
	MaxConcurrentVideoDownloads     int    `json:"maxConcurrentVideoDownloads"`
	MaxConcurrentEnrichmentRequests int    `json:"maxConcurrentEnrichmentRequests"`
	MaxConcurrentLyricsRequests     int    `json:"maxConcurrentLyricsRequests"`
	ThrottleOnYTDLPBotErrors        bool   `json:"throttleOnYtdlpBotErrors"`
}

type ToolStatus struct {
	YTDLP  bool `json:"ytDlp"`
	FFmpeg bool `json:"ffmpeg"`
	AI     bool `json:"ai"`
}

// ToolReadiness is the safe, user-facing result of the most recent external
// tool probe. It intentionally contains no executable path or command output.
type ToolReadiness struct {
	ToolName      string `json:"tool"`
	Status        string `json:"status"`
	Version       string `json:"version,omitempty"`
	Error         string `json:"error,omitempty"`
	Remediation   string `json:"remediation,omitempty"`
	CorrelationID string `json:"correlationId,omitempty"`
}

type BuildInfo struct {
	AppVersion                 string `json:"appVersion"`
	BuildNumber                string `json:"buildNumber"`
	GitCommit                  string `json:"gitCommit"`
	BuildTime                  string `json:"buildTime"`
	ReleaseChannel             string `json:"releaseChannel"`
	GoVersion                  string `json:"goVersion"`
	SettingsSchemaVersion      int    `json:"settingsSchemaVersion"`
	CatalogSchemaVersion       int    `json:"catalogSchemaVersion"`
	LibraryCacheSchemaVersion  int    `json:"libraryCacheSchemaVersion"`
	PlaylistSchemaVersion      int    `json:"playlistSchemaVersion"`
	ImportHistorySchemaVersion int    `json:"importHistorySchemaVersion"`
	TrackMetadataSchemaVersion int    `json:"trackMetadataSchemaVersion"`
}

type UpdateInfo struct {
	ManifestURL    string    `json:"manifestUrl,omitempty"`
	CurrentVersion string    `json:"currentVersion,omitempty"`
	LatestVersion  string    `json:"latestVersion,omitempty"`
	ReleaseDate    time.Time `json:"releaseDate,omitempty"`
	ReleaseNotes   string    `json:"releaseNotes,omitempty"`
	DownloadURL    string    `json:"downloadUrl,omitempty"`
	Platform       string    `json:"platform,omitempty"`
	Available      bool      `json:"available"`
	Mandatory      bool      `json:"mandatory"`
	MinimumVersion string    `json:"minimumVersion,omitempty"`
	CheckedAt      time.Time `json:"checkedAt,omitempty"`
	Status         string    `json:"status,omitempty"`
	Error          string    `json:"error,omitempty"`
}

type DiagnosticsInfo struct {
	LogPath        string    `json:"logPath,omitempty"`
	LastExportPath string    `json:"lastExportPath,omitempty"`
	LastExportedAt time.Time `json:"lastExportedAt,omitempty"`
	BundleStatus   string    `json:"bundleStatus,omitempty"`
	BundleMessage  string    `json:"bundleMessage,omitempty"`
}

type LibraryStats struct {
	TrackCount  int `json:"trackCount"`
	ArtistCount int `json:"artistCount"`
	AlbumCount  int `json:"albumCount"`
	JobCount    int `json:"jobCount"`
	PendingJobs int `json:"pendingJobs"`
	FailedJobs  int `json:"failedJobs"`
}

type LibraryHealth struct {
	TotalTracks       int `json:"totalTracks"`
	ReadyTracks       int `json:"readyTracks"`
	UnprocessedTracks int `json:"unprocessedTracks"`
	MissingAudio      int `json:"missingAudio"`
	MissingLyrics     int `json:"missingLyrics"`
	MissingTimed      int `json:"missingTimed"`
	MissingMetadata   int `json:"missingMetadata"`
}

type WorkerClassStats struct {
	Capacity         int    `json:"capacity"`
	Active           int    `json:"active"`
	Queued           int    `json:"queued"`
	Blocked          bool   `json:"blocked"`
	BlockedReason    string `json:"blockedReason,omitempty"`
	SharedCapacityOf string `json:"sharedCapacityOf,omitempty"`
}

type WorkerThrottleState struct {
	Active bool      `json:"active"`
	Until  time.Time `json:"until,omitempty"`
	Reason string    `json:"reason,omitempty"`
}

type WorkerStats struct {
	ActiveWorkers int `json:"activeWorkers"`
	MaxWorkers    int `json:"maxWorkers"`
	VideoActive   int `json:"videoActive"`
	VideoQueued   int `json:"videoQueued"`

	Audio         WorkerClassStats    `json:"audio"`
	Video         WorkerClassStats    `json:"video"`
	Enrichment    WorkerClassStats    `json:"enrichment"`
	Lyrics        WorkerClassStats    `json:"lyrics"`
	Artwork       WorkerClassStats    `json:"artwork"`
	Throttle      WorkerThrottleState `json:"throttle"`
	BlockedReason string              `json:"blockedReason,omitempty"`
}

type AppState struct {
	Settings         PublicSettings           `json:"settings"`
	Stats            LibraryStats             `json:"stats"`
	Health           LibraryHealth            `json:"health"`
	Workers          WorkerStats              `json:"workers"`
	GenreBuckets     []FacetBucket            `json:"genreBuckets"`
	YearBuckets      []FacetBucket            `json:"yearBuckets"`
	BuildInfo        BuildInfo                `json:"buildInfo"`
	UpdateInfo       UpdateInfo               `json:"updateInfo"`
	Diagnostics      DiagnosticsInfo          `json:"diagnostics"`
	AIStatus         string                   `json:"aiStatus"`
	Jobs             []Job                    `json:"jobs"`
	ImportHistory    []ImportHistoryEntry     `json:"importHistory"`
	LibraryTracks    []TrackRecord            `json:"libraryTracks"`
	RecentTracks     []TrackRecord            `json:"recentTracks"`
	Playlists        []Playlist               `json:"playlists"`
	Playback         PlaybackState            `json:"playback"`
	PromptFiles      []PromptInfo             `json:"promptFiles"`
	ToolStatus       ToolStatus               `json:"toolStatus"`
	ToolReadiness    map[string]ToolReadiness `json:"toolReadiness"`
	RootInfo         RootInfo                 `json:"rootInfo"`
	WindowFullscreen bool                     `json:"windowFullscreen"`
}

type RootInfo struct {
	LibraryRoot string `json:"libraryRoot"`
	AppDataDir  string `json:"appDataDir"`
	IncomingDir string `json:"incomingDir"`
	LibraryDir  string `json:"libraryDir"`
	CacheDir    string `json:"cacheDir"`
}

type PromptInfo struct {
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
}

type FacetBucket struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type ImportHistoryEntry struct {
	URL         string    `json:"url"`
	CompletedAt time.Time `json:"completedAt"`
}

type JobStageStatuses struct {
	Download    string `json:"download,omitempty"`
	Metadata    string `json:"metadata,omitempty"`
	MusicBrainz string `json:"musicBrainz,omitempty"`
	Lyrics      string `json:"lyrics,omitempty"`
	Artwork     string `json:"artwork,omitempty"`
	Video       string `json:"video,omitempty"`
	Finalize    string `json:"finalize,omitempty"`
}

type Job struct {
	ID                     string           `json:"id"`
	Kind                   string           `json:"kind"`
	Input                  string           `json:"input"`
	SourceRoot             string           `json:"sourceRoot,omitempty"`
	DownloadVideo          bool             `json:"downloadVideo,omitempty"`
	TrackID                string           `json:"trackId,omitempty"`
	UserContext            string           `json:"userContext,omitempty"`
	ParentJobID            string           `json:"parentJobId,omitempty"`
	PlaylistURL            string           `json:"playlistUrl,omitempty"`
	PlaylistTitle          string           `json:"playlistTitle,omitempty"`
	PlaylistChannel        string           `json:"playlistChannel,omitempty"`
	PlaylistTotalItems     int              `json:"playlistTotalItems,omitempty"`
	PlaylistProcessedItems int              `json:"playlistProcessedItems,omitempty"`
	PlaylistFailedItems    int              `json:"playlistFailedItems,omitempty"`
	PlaylistIndex          int              `json:"playlistIndex,omitempty"`
	PlaylistCurrentIndex   int              `json:"playlistCurrentIndex,omitempty"`
	PlaylistCurrentTitle   string           `json:"playlistCurrentTitle,omitempty"`
	PlaylistItemTitle      string           `json:"playlistItemTitle,omitempty"`
	Status                 string           `json:"status"`
	Detail                 string           `json:"detail"`
	Error                  string           `json:"error,omitempty"`
	OutputBundle           string           `json:"outputBundle,omitempty"`
	StageStatuses          JobStageStatuses `json:"stageStatuses,omitempty"`
	DownloadProgress       int              `json:"downloadProgress,omitempty"`
	MetadataProgress       int              `json:"metadataProgress,omitempty"`
	LyricsProgress         int              `json:"lyricsProgress,omitempty"`
	ResultTitle            string           `json:"resultTitle,omitempty"`
	ResultArtist           string           `json:"resultArtist,omitempty"`
	ResultAlbum            string           `json:"resultAlbum,omitempty"`
	CreatedAt              time.Time        `json:"createdAt"`
	StartedAt              time.Time        `json:"startedAt,omitempty"`
	FinishedAt             time.Time        `json:"finishedAt,omitempty"`
}

type TrackRecord struct {
	ID                     string        `json:"id"`
	Artist                 string        `json:"artist"`
	Album                  string        `json:"album"`
	Title                  string        `json:"title"`
	Genre                  string        `json:"genre"`
	Year                   string        `json:"year"`
	LyricsIncluded         bool          `json:"lyricsIncluded"`
	Confidence             string        `json:"confidence,omitempty"`
	BundlePath             string        `json:"bundlePath"`
	StorageDir             string        `json:"storageDir"`
	AudioPath              string        `json:"audioPath"`
	LyricsPath             string        `json:"lyricsPath"`
	LRCPath                string        `json:"lrcPath"`
	ArtworkPath            string        `json:"artworkPath"`
	VideoPath              string        `json:"videoPath"`
	ArtworkDataURL         string        `json:"artworkDataUrl,omitempty"`
	ArtworkMediaURL        string        `json:"artworkMediaUrl,omitempty"`
	ArtworkURL             string        `json:"artworkUrl,omitempty"`
	VideoURL               string        `json:"videoUrl,omitempty"`
	MetadataPath           string        `json:"metadataPath"`
	MetadataConfidence     string        `json:"metadataConfidence,omitempty"`
	EnrichmentConfidence   string        `json:"enrichmentConfidence,omitempty"`
	ReleaseType            string        `json:"releaseType,omitempty"`
	ISRC                   string        `json:"isrc,omitempty"`
	ArtistLinks            MetadataLinks `json:"artistLinks,omitempty"`
	AlbumLinks             MetadataLinks `json:"albumLinks,omitempty"`
	SongLinks              MetadataLinks `json:"songLinks,omitempty"`
	ArtistTrivia           []string      `json:"artistTrivia,omitempty"`
	AlbumTrivia            []string      `json:"albumTrivia,omitempty"`
	SongTrivia             []string      `json:"songTrivia,omitempty"`
	SongMeaning            string        `json:"songMeaning,omitempty"`
	Tidbits                []string      `json:"tidbits,omitempty"`
	Sources                []string      `json:"sources,omitempty"`
	MetadataNotes          string        `json:"metadataNotes,omitempty"`
	LyricsConfidence       string        `json:"lyricsConfidence,omitempty"`
	LyricsSource           string        `json:"lyricsSource,omitempty"`
	LyricsSourceLoc        string        `json:"lyricsSourceLoc,omitempty"`
	TimedLyricsConfidence  string        `json:"timedLyricsConfidence,omitempty"`
	TimedLyricsGranularity string        `json:"timedLyricsGranularity,omitempty"`
	HasTimedLyrics         bool          `json:"hasTimedLyrics"`
	SourceTitle            string        `json:"sourceTitle,omitempty"`
	SourceChannel          string        `json:"sourceChannel,omitempty"`
	AIProvider             string        `json:"aiProvider,omitempty"`
	AIModel                string        `json:"aiModel,omitempty"`
	AIRan                  bool          `json:"aiRan,omitempty"`
	AIStatus               string        `json:"aiStatus,omitempty"`
	AIMessage              string        `json:"aiMessage,omitempty"`
	AIContext              SongAIContext `json:"aiContext,omitempty"`
	GeneratedAt            time.Time     `json:"generatedAt,omitempty"`
	SourceKind             string        `json:"sourceKind"`
	SourceRef              string        `json:"sourceRef"`
	DurationSeconds        *int          `json:"durationSeconds,omitempty"`
	MetadataSize           int64         `json:"metadataSize,omitempty"`
	MetadataModTime        time.Time     `json:"metadataModTime,omitempty"`
	Hash                   string        `json:"hash"`
	CreatedAt              time.Time     `json:"createdAt"`
}

type Playlist struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	TrackIDs    []string  `json:"trackIds"`
}

type PlaybackState struct {
	CurrentTrackID   string   `json:"currentTrackId"`
	CurrentTrackPath string   `json:"currentTrackPath"`
	Queue            []string `json:"queue"`
	QueueSource      string   `json:"queueSource"`
	IsPlaying        bool     `json:"isPlaying"`
	IsLoading        bool     `json:"isLoading"`
	CurrentTime      float64  `json:"currentTime"`
	Duration         float64  `json:"duration"`
	Volume           float64  `json:"volume"`
	Muted            bool     `json:"muted"`
	ShuffleEnabled   bool     `json:"shuffleEnabled"`
	RepeatMode       string   `json:"repeatMode"`
	Error            string   `json:"error"`
}

type TrackFileState struct {
	Label  string `json:"label"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

type MediaFileInfo struct {
	URL           string    `json:"url"`
	MimeType      string    `json:"mimeType"`
	Reachable     bool      `json:"reachable"`
	StatusCode    int       `json:"statusCode"`
	ContentLength int64     `json:"contentLength"`
	ExpiresAt     time.Time `json:"expiresAt,omitempty"`
}

type MetadataLinks struct {
	OfficialWebsite string `json:"officialWebsite,omitempty"`
	Spotify         string `json:"spotify,omitempty"`
	AppleMusic      string `json:"appleMusic,omitempty"`
	YouTube         string `json:"youtube,omitempty"`
	YouTubeMusic    string `json:"youtubeMusic,omitempty"`
	Instagram       string `json:"instagram,omitempty"`
	X               string `json:"x,omitempty"`
	Facebook        string `json:"facebook,omitempty"`
	Bandcamp        string `json:"bandcamp,omitempty"`
	SoundCloud      string `json:"soundcloud,omitempty"`
	Wikipedia       string `json:"wikipedia,omitempty"`
	MusicBrainz     string `json:"musicBrainz,omitempty"`
	Genius          string `json:"genius,omitempty"`
}

type TrackPreview struct {
	Track                  TrackRecord      `json:"track"`
	LyricsText             string           `json:"lyricsText"`
	TimedLyricsText        string           `json:"timedLyricsText"`
	FileStates             []TrackFileState `json:"fileStates"`
	Title                  string           `json:"title"`
	Artist                 string           `json:"artist"`
	Album                  string           `json:"album"`
	TrackNumber            *int             `json:"trackNumber,omitempty"`
	Year                   *int             `json:"year,omitempty"`
	Genre                  string           `json:"genre"`
	DurationSeconds        *int             `json:"durationSeconds,omitempty"`
	Confidence             string           `json:"confidence,omitempty"`
	MetadataConfidence     string           `json:"metadataConfidence,omitempty"`
	EnrichmentConfidence   string           `json:"enrichmentConfidence,omitempty"`
	ReleaseType            string           `json:"releaseType,omitempty"`
	ISRC                   string           `json:"isrc,omitempty"`
	ArtistLinks            MetadataLinks    `json:"artistLinks,omitempty"`
	AlbumLinks             MetadataLinks    `json:"albumLinks,omitempty"`
	SongLinks              MetadataLinks    `json:"songLinks,omitempty"`
	ArtistTrivia           []string         `json:"artistTrivia,omitempty"`
	AlbumTrivia            []string         `json:"albumTrivia,omitempty"`
	SongTrivia             []string         `json:"songTrivia,omitempty"`
	SongMeaning            string           `json:"songMeaning,omitempty"`
	Tidbits                []string         `json:"tidbits,omitempty"`
	Sources                []string         `json:"sources,omitempty"`
	MetadataNotes          string           `json:"metadataNotes,omitempty"`
	SourceURL              string           `json:"sourceUrl"`
	SourceProvider         string           `json:"sourceProvider"`
	SourceVideoID          string           `json:"sourceVideoId"`
	SourceTitle            string           `json:"sourceTitle"`
	SourceChannel          string           `json:"sourceChannel"`
	AIProvider             string           `json:"aiProvider"`
	AIModel                string           `json:"aiModel"`
	AIRan                  bool             `json:"aiRan"`
	AIStatus               string           `json:"aiStatus,omitempty"`
	AIMessage              string           `json:"aiMessage,omitempty"`
	AIContext              SongAIContext    `json:"aiContext,omitempty"`
	GeneratedAt            time.Time        `json:"generatedAt"`
	LyricsConfidence       string           `json:"lyricsConfidence,omitempty"`
	LyricsSource           string           `json:"lyricsSource,omitempty"`
	LyricsSourceLoc        string           `json:"lyricsSourceLoc,omitempty"`
	TimedLyricsConfidence  string           `json:"timedLyricsConfidence,omitempty"`
	TimedLyricsSource      string           `json:"timedLyricsSource,omitempty"`
	TimedLyricsGranularity string           `json:"timedLyricsGranularity,omitempty"`
	ArtworkPath            string           `json:"artworkPath,omitempty"`
	ArtworkDataURL         string           `json:"artworkDataUrl,omitempty"`
	ArtworkMediaURL        string           `json:"artworkMediaUrl,omitempty"`
	VideoPath              string           `json:"videoPath,omitempty"`
	VideoURL               string           `json:"videoUrl,omitempty"`
}

type BundleManifest struct {
	Version        int       `json:"version"`
	ID             string    `json:"id"`
	Artist         string    `json:"artist"`
	Album          string    `json:"album"`
	Title          string    `json:"title"`
	Genre          string    `json:"genre"`
	Year           string    `json:"year"`
	Lyrics         string    `json:"lyrics"`
	SourceKind     string    `json:"sourceKind"`
	SourceRef      string    `json:"sourceRef"`
	OriginalName   string    `json:"originalName"`
	AudioEntry     string    `json:"audioEntry"`
	MetadataEntry  string    `json:"metadataEntry"`
	LyricsEntry    string    `json:"lyricsEntry"`
	Hash           string    `json:"hash"`
	CreatedAt      time.Time `json:"createdAt"`
	EnrichmentNote string    `json:"enrichmentNote"`
}

type EnrichmentResult struct {
	Artist     string  `json:"artist"`
	Album      string  `json:"album"`
	Title      string  `json:"title"`
	Genre      string  `json:"genre"`
	Year       string  `json:"year"`
	Lyrics     string  `json:"lyrics"`
	Notes      string  `json:"notes"`
	Confidence float64 `json:"confidence"`
}

type storedSettings struct {
	PublicSettings
	APIKey        string `json:"apiKey"`
	SchemaVersion int    `json:"schemaVersion,omitempty"`
}

type catalogFile struct {
	Version int           `json:"version"`
	Tracks  []TrackRecord `json:"tracks"`
	Jobs    []Job         `json:"jobs"`
}

type playlistFile struct {
	Version   int        `json:"version"`
	Playlists []Playlist `json:"playlists"`
}

type importHistoryFile struct {
	Version int                  `json:"version"`
	Entries []ImportHistoryEntry `json:"entries"`
}
