package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	LibraryManifestFormat        = "melodex.library-manifest"
	LibraryManifestSchemaVersion = 1
)

var (
	ErrLibraryManifestChecksumMismatch = errors.New("library manifest checksum mismatch")
	ErrLibraryManifestConflicts        = errors.New("library manifest import has unresolved conflicts")
	ErrLibraryManifestPathRequired     = errors.New("library manifest target root is required")
)

// LibraryManifest is the portable, metadata-only representation used by local
// backup/restore and future sync code. It deliberately does not embed
// TrackRecord or PublicSettings: those types contain media URLs, local tool
// paths, and fields that must never cross an export boundary.
type LibraryManifest struct {
	Format           string                        `json:"format"`
	SchemaVersion    int                           `json:"schemaVersion"`
	ExportedAt       time.Time                     `json:"exportedAt"`
	ManifestChecksum string                        `json:"manifestChecksum"`
	Exclusions       LibraryManifestExclusions     `json:"exclusions"`
	Tracks           []LibraryManifestTrack        `json:"tracks"`
	Playlists        []LibraryManifestPlaylist     `json:"playlists"`
	Ratings          []LibraryManifestRating       `json:"ratings,omitempty"`
	Favorites        []string                      `json:"favorites,omitempty"`
	History          []LibraryManifestHistoryEntry `json:"history,omitempty"`
	Settings         LibraryManifestSettings       `json:"settings"`
	ProfileNotes     map[string]string             `json:"profileNotes,omitempty"`
}

// LibraryManifestExclusions is serialized as part of every manifest so a
// consumer can verify the privacy contract without relying on documentation.
// The booleans are intentionally not configurable by callers.
type LibraryManifestExclusions struct {
	APIKeysExcluded       bool     `json:"apiKeysExcluded"`
	SecretsExcluded       bool     `json:"secretsExcluded"`
	MediaBinariesExcluded bool     `json:"mediaBinariesExcluded"`
	LyricsTextExcluded    bool     `json:"lyricsTextExcluded"`
	ArtworkBytesExcluded  bool     `json:"artworkBytesExcluded"`
	ExcludedFields        []string `json:"excludedFields"`
}

type LibraryManifestExportSource struct {
	LibraryRoot  string
	Tracks       []TrackRecord
	Playlists    []Playlist
	Ratings      map[string]int
	Favorites    []string
	History      []ImportHistoryEntry
	ProfileNotes map[string]string
	Settings     PublicSettings
}

type LibraryManifestExportOptions struct {
	// ExportedAt is useful for deterministic backups and tests. A zero value
	// uses the current UTC time.
	ExportedAt time.Time
}

type LibraryManifestTrack struct {
	ID         string                          `json:"id"`
	Metadata   LibraryManifestTrackMetadata    `json:"metadata"`
	Paths      LibraryManifestTrackPaths       `json:"paths"`
	Lyrics     LibraryManifestLyricsReference  `json:"lyrics"`
	Artwork    LibraryManifestArtworkReference `json:"artwork"`
	Provenance LibraryManifestProvenance       `json:"provenance"`
}

type LibraryManifestTrackMetadata struct {
	Title                string        `json:"title"`
	Artist               string        `json:"artist"`
	Album                string        `json:"album"`
	Genre                string        `json:"genre"`
	Year                 string        `json:"year,omitempty"`
	DurationSeconds      *int          `json:"durationSeconds,omitempty"`
	Confidence           string        `json:"confidence,omitempty"`
	MetadataConfidence   string        `json:"metadataConfidence,omitempty"`
	EnrichmentConfidence string        `json:"enrichmentConfidence,omitempty"`
	ReleaseType          string        `json:"releaseType,omitempty"`
	ISRC                 string        `json:"isrc,omitempty"`
	ArtistLinks          MetadataLinks `json:"artistLinks,omitempty"`
	AlbumLinks           MetadataLinks `json:"albumLinks,omitempty"`
	SongLinks            MetadataLinks `json:"songLinks,omitempty"`
	ArtistTrivia         []string      `json:"artistTrivia,omitempty"`
	AlbumTrivia          []string      `json:"albumTrivia,omitempty"`
	SongTrivia           []string      `json:"songTrivia,omitempty"`
	SongMeaning          string        `json:"songMeaning,omitempty"`
	Tidbits              []string      `json:"tidbits,omitempty"`
	Sources              []string      `json:"sources,omitempty"`
	Notes                string        `json:"notes,omitempty"`
}

type LibraryManifestTrackPaths struct {
	Metadata    LibraryManifestPathReference `json:"metadata"`
	StorageDir  LibraryManifestPathReference `json:"storageDir,omitempty"`
	Bundle      LibraryManifestPathReference `json:"bundle,omitempty"`
	Audio       LibraryManifestPathReference `json:"audio,omitempty"`
	TimedLyrics LibraryManifestPathReference `json:"timedLyrics,omitempty"`
	Video       LibraryManifestPathReference `json:"video,omitempty"`
}

// LibraryManifestPathReference contains a relative reference only. It never
// contains file bytes. External paths are represented by a digest so an
// export cannot leak an absolute path while still allowing import to report a
// deterministic remapping problem.
type LibraryManifestPathReference struct {
	RelativePath string `json:"relativePath,omitempty"`
	External     bool   `json:"external,omitempty"`
	PathDigest   string `json:"pathDigest,omitempty"`
}

type LibraryManifestLyricsReference struct {
	Path             LibraryManifestPathReference `json:"path,omitempty"`
	Available        bool                         `json:"available"`
	TimedAvailable   bool                         `json:"timedAvailable"`
	Confidence       string                       `json:"confidence,omitempty"`
	TimedConfidence  string                       `json:"timedConfidence,omitempty"`
	TimedGranularity string                       `json:"timedGranularity,omitempty"`
	Source           string                       `json:"source,omitempty"`
	SourceLocation   string                       `json:"sourceLocation,omitempty"`
}

type LibraryManifestArtworkReference struct {
	Path           LibraryManifestPathReference `json:"path,omitempty"`
	Source         string                       `json:"source,omitempty"`
	ReleaseID      string                       `json:"releaseId,omitempty"`
	ReleaseGroupID string                       `json:"releaseGroupId,omitempty"`
	URL            string                       `json:"url,omitempty"`
	FetchedAt      time.Time                    `json:"fetchedAt,omitempty"`
}

type LibraryManifestProvenance struct {
	SourceKind           string    `json:"sourceKind,omitempty"`
	SourceRef            string    `json:"sourceRef,omitempty"`
	SourceURL            string    `json:"sourceUrl,omitempty"`
	SourceTitle          string    `json:"sourceTitle,omitempty"`
	SourceChannel        string    `json:"sourceChannel,omitempty"`
	Sources              []string  `json:"sources,omitempty"`
	MetadataConfidence   string    `json:"metadataConfidence,omitempty"`
	EnrichmentConfidence string    `json:"enrichmentConfidence,omitempty"`
	AIProvider           string    `json:"aiProvider,omitempty"`
	AIModel              string    `json:"aiModel,omitempty"`
	AIRan                bool      `json:"aiRan,omitempty"`
	GeneratedAt          time.Time `json:"generatedAt,omitempty"`
}

type LibraryManifestSettings struct {
	Provider                        string `json:"provider,omitempty"`
	AIModel                         string `json:"aiModel,omitempty"`
	DownloadMusicVideo              bool   `json:"downloadMusicVideo"`
	KeepOriginalAudio               bool   `json:"keepOriginalAudio"`
	MaxConcurrentDownloads          int    `json:"maxConcurrentDownloads,omitempty"`
	MaxConcurrentVideoDownloads     int    `json:"maxConcurrentVideoDownloads,omitempty"`
	MaxConcurrentEnrichmentRequests int    `json:"maxConcurrentEnrichmentRequests,omitempty"`
	MaxConcurrentLyricsRequests     int    `json:"maxConcurrentLyricsRequests,omitempty"`
	ThrottleOnYTDLPBotErrors        bool   `json:"throttleOnYtdlpBotErrors"`
}

type LibraryManifestPlaylist struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	TrackIDs    []string  `json:"trackIds"`
}

type LibraryManifestRating struct {
	TrackID string `json:"trackId"`
	Rating  int    `json:"rating"`
}

type LibraryManifestHistoryEntry struct {
	URL         string    `json:"url,omitempty"`
	CompletedAt time.Time `json:"completedAt"`
}

// LibraryManifestSourceFromAppState adapts the state already exposed by the
// application without making the manifest depend on app lifecycle internals.
// Ratings and favorites remain empty until the application has those fields.
func LibraryManifestSourceFromAppState(state AppState) LibraryManifestExportSource {
	return LibraryManifestExportSource{
		LibraryRoot: state.RootInfo.LibraryRoot,
		Tracks:      append([]TrackRecord(nil), state.LibraryTracks...),
		Playlists:   append([]Playlist(nil), state.Playlists...),
		History:     append([]ImportHistoryEntry(nil), state.ImportHistory...),
		Settings:    state.Settings,
	}
}

func BuildLibraryManifest(source LibraryManifestExportSource, options LibraryManifestExportOptions) (LibraryManifest, error) {
	exportedAt := options.ExportedAt
	if exportedAt.IsZero() {
		exportedAt = time.Now().UTC()
	} else {
		exportedAt = exportedAt.UTC()
	}

	manifest := LibraryManifest{
		Format:        LibraryManifestFormat,
		SchemaVersion: LibraryManifestSchemaVersion,
		ExportedAt:    exportedAt,
		Exclusions:    defaultLibraryManifestExclusions(),
		Tracks:        make([]LibraryManifestTrack, 0, len(source.Tracks)),
		Playlists:     make([]LibraryManifestPlaylist, 0, len(source.Playlists)),
		Ratings:       make([]LibraryManifestRating, 0, len(source.Ratings)),
		Favorites:     uniqueSortedStrings(source.Favorites),
		History:       make([]LibraryManifestHistoryEntry, 0, len(source.History)),
		Settings:      safeLibraryManifestSettings(source.Settings),
		ProfileNotes:  sanitizeManifestMap(source.ProfileNotes),
	}

	seenTracks := make(map[string]struct{}, len(source.Tracks))
	for _, track := range source.Tracks {
		if strings.TrimSpace(track.ID) == "" {
			return LibraryManifest{}, errors.New("library manifest track is missing an id")
		}
		if _, exists := seenTracks[track.ID]; exists {
			return LibraryManifest{}, fmt.Errorf("duplicate library manifest track id %q", track.ID)
		}
		seenTracks[track.ID] = struct{}{}
		manifestTrack, err := buildLibraryManifestTrack(source.LibraryRoot, track)
		if err != nil {
			return LibraryManifest{}, fmt.Errorf("track %q: %w", track.ID, err)
		}
		manifest.Tracks = append(manifest.Tracks, manifestTrack)
	}
	sort.SliceStable(manifest.Tracks, func(i, j int) bool { return manifest.Tracks[i].ID < manifest.Tracks[j].ID })

	seenPlaylists := make(map[string]struct{}, len(source.Playlists))
	for _, playlist := range source.Playlists {
		if strings.TrimSpace(playlist.ID) == "" {
			return LibraryManifest{}, errors.New("library manifest playlist is missing an id")
		}
		if _, exists := seenPlaylists[playlist.ID]; exists {
			return LibraryManifest{}, fmt.Errorf("duplicate library manifest playlist id %q", playlist.ID)
		}
		seenPlaylists[playlist.ID] = struct{}{}
		manifest.Playlists = append(manifest.Playlists, LibraryManifestPlaylist{
			ID:          playlist.ID,
			Name:        playlist.Name,
			Description: sanitizeManifestText(playlist.Description),
			CreatedAt:   playlist.CreatedAt,
			UpdatedAt:   playlist.UpdatedAt,
			TrackIDs:    append([]string(nil), playlist.TrackIDs...),
		})
	}

	for trackID, rating := range source.Ratings {
		if strings.TrimSpace(trackID) == "" {
			return LibraryManifest{}, errors.New("library manifest rating is missing a track id")
		}
		if rating < 0 || rating > 5 {
			return LibraryManifest{}, fmt.Errorf("rating for track %q must be between 0 and 5", trackID)
		}
		manifest.Ratings = append(manifest.Ratings, LibraryManifestRating{TrackID: trackID, Rating: rating})
	}
	sort.Slice(manifest.Ratings, func(i, j int) bool { return manifest.Ratings[i].TrackID < manifest.Ratings[j].TrackID })

	for _, entry := range source.History {
		manifest.History = append(manifest.History, LibraryManifestHistoryEntry{
			URL:         sanitizeManifestURL(entry.URL),
			CompletedAt: entry.CompletedAt,
		})
	}

	return normalizeLibraryManifest(manifest)
}

func MarshalLibraryManifest(manifest LibraryManifest) ([]byte, error) {
	manifest, err := normalizeLibraryManifest(manifest)
	if err != nil {
		return nil, err
	}
	manifest.ManifestChecksum = ""
	unsigned, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal unsigned library manifest: %w", err)
	}
	digest := sha256.Sum256(unsigned)
	manifest.ManifestChecksum = hex.EncodeToString(digest[:])
	return json.MarshalIndent(manifest, "", "  ")
}

func UnmarshalLibraryManifest(data []byte) (LibraryManifest, error) {
	var manifest LibraryManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return LibraryManifest{}, fmt.Errorf("decode library manifest: %w", err)
	}
	manifest, err := normalizeLibraryManifest(manifest)
	if err != nil {
		return LibraryManifest{}, err
	}
	providedChecksum := strings.TrimSpace(manifest.ManifestChecksum)
	if providedChecksum != "" {
		manifest.ManifestChecksum = ""
		unsigned, err := json.Marshal(manifest)
		if err != nil {
			return LibraryManifest{}, fmt.Errorf("marshal library manifest for checksum: %w", err)
		}
		digest := sha256.Sum256(unsigned)
		if !strings.EqualFold(providedChecksum, hex.EncodeToString(digest[:])) {
			return LibraryManifest{}, ErrLibraryManifestChecksumMismatch
		}
		manifest.ManifestChecksum = providedChecksum
	}
	return manifest, nil
}

func WriteLibraryManifestFile(path string, manifest LibraryManifest) error {
	data, err := MarshalLibraryManifest(manifest)
	if err != nil {
		return err
	}
	return writeAtomicFile(path, data, 0o600)
}

func ReadLibraryManifestFile(path string) (LibraryManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LibraryManifest{}, err
	}
	return UnmarshalLibraryManifest(data)
}

type LibraryManifestPathMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type LibraryManifestPathResolution struct {
	TrackID      string `json:"trackId"`
	Field        string `json:"field"`
	RelativePath string `json:"relativePath,omitempty"`
	ResolvedPath string `json:"resolvedPath,omitempty"`
	Error        string `json:"error,omitempty"`
}

func RemapLibraryManifestRelativePath(relativePath string, mappings []LibraryManifestPathMapping) (string, error) {
	relativePath, err := normalizeLibraryManifestRelativePath(relativePath)
	if err != nil {
		return "", err
	}
	best := -1
	bestFrom := ""
	bestTo := ""
	for _, mapping := range mappings {
		from, err := normalizeMappingPrefix(mapping.From)
		if err != nil {
			return "", fmt.Errorf("invalid path mapping %q: %w", mapping.From, err)
		}
		to, err := normalizeMappingPrefix(mapping.To)
		if err != nil {
			return "", fmt.Errorf("invalid path mapping target %q: %w", mapping.To, err)
		}
		if from != "" && relativePath != from && !strings.HasPrefix(relativePath, from+"/") {
			continue
		}
		if len(from) > best {
			best = len(from)
			bestFrom = from
			bestTo = to
		}
	}
	if best < 0 {
		return relativePath, nil
	}
	suffix := strings.TrimPrefix(relativePath, bestFrom)
	suffix = strings.TrimPrefix(suffix, "/")
	remapped := bestTo
	if suffix != "" {
		if remapped != "" {
			remapped += "/"
		}
		remapped += suffix
	}
	return normalizeLibraryManifestRelativePath(remapped)
}

func ResolveLibraryManifestPath(reference LibraryManifestPathReference, targetRoot string, mappings []LibraryManifestPathMapping) (string, error) {
	if reference.External {
		return "", fmt.Errorf("external path reference %s cannot be remapped", reference.PathDigest)
	}
	if strings.TrimSpace(targetRoot) == "" {
		return "", ErrLibraryManifestPathRequired
	}
	relativePath, err := RemapLibraryManifestRelativePath(reference.RelativePath, mappings)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(targetRoot)
	if err != nil {
		return "", fmt.Errorf("resolve target root: %w", err)
	}
	candidate := filepath.Join(root, filepath.FromSlash(relativePath))
	inside, err := filepath.Rel(root, candidate)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) || filepath.IsAbs(inside) {
		return "", fmt.Errorf("resolved path escapes target root")
	}
	return candidate, nil
}

type LibraryManifestConflictPolicy string

const (
	LibraryManifestReportConflictsOnly LibraryManifestConflictPolicy = "report-only"
	LibraryManifestKeepExisting        LibraryManifestConflictPolicy = "keep-existing"
	LibraryManifestPreferIncoming      LibraryManifestConflictPolicy = "prefer-incoming"
)

type LibraryManifestImportOptions struct {
	DryRun         bool
	TargetRoot     string
	PathMappings   []LibraryManifestPathMapping
	ConflictPolicy LibraryManifestConflictPolicy
}

type LibraryManifestImportTarget struct {
	Tracks       []LibraryManifestTrack
	Playlists    []LibraryManifestPlaylist
	Ratings      []LibraryManifestRating
	Favorites    []string
	History      []LibraryManifestHistoryEntry
	Settings     LibraryManifestSettings
	ProfileNotes map[string]string
}

type LibraryManifestConflict struct {
	Entity              string `json:"entity"`
	EntityID            string `json:"entityId"`
	Field               string `json:"field,omitempty"`
	Kind                string `json:"kind"`
	ExistingFingerprint string `json:"existingFingerprint,omitempty"`
	IncomingFingerprint string `json:"incomingFingerprint,omitempty"`
	Detail              string `json:"detail"`
	Resolution          string `json:"resolution"`
}

type LibraryManifestImportPlan struct {
	DryRun            bool                            `json:"dryRun"`
	TargetRoot        string                          `json:"targetRoot,omitempty"`
	ConflictPolicy    LibraryManifestConflictPolicy   `json:"conflictPolicy"`
	TracksToCreate    []string                        `json:"tracksToCreate"`
	TracksToUpdate    []string                        `json:"tracksToUpdate"`
	TracksToSkip      []string                        `json:"tracksToSkip"`
	PlaylistsToCreate []string                        `json:"playlistsToCreate"`
	PlaylistsToUpdate []string                        `json:"playlistsToUpdate"`
	PlaylistsToSkip   []string                        `json:"playlistsToSkip"`
	PathResolutions   []LibraryManifestPathResolution `json:"pathResolutions"`
	Conflicts         []LibraryManifestConflict       `json:"conflicts,omitempty"`
	manifest          LibraryManifest
	pathMappings      []LibraryManifestPathMapping
}

func PlanLibraryManifestImport(manifest LibraryManifest, target LibraryManifestImportTarget, options LibraryManifestImportOptions) (LibraryManifestImportPlan, error) {
	manifest, err := normalizeLibraryManifest(manifest)
	if err != nil {
		return LibraryManifestImportPlan{}, err
	}
	policy := options.ConflictPolicy
	if policy == "" {
		policy = LibraryManifestReportConflictsOnly
	}
	if policy != LibraryManifestReportConflictsOnly && policy != LibraryManifestKeepExisting && policy != LibraryManifestPreferIncoming {
		return LibraryManifestImportPlan{}, fmt.Errorf("unsupported library manifest conflict policy %q", policy)
	}
	plan := LibraryManifestImportPlan{
		DryRun:         options.DryRun,
		TargetRoot:     options.TargetRoot,
		ConflictPolicy: policy,
		TracksToCreate: make([]string, 0), TracksToUpdate: make([]string, 0), TracksToSkip: make([]string, 0),
		PlaylistsToCreate: make([]string, 0), PlaylistsToUpdate: make([]string, 0), PlaylistsToSkip: make([]string, 0),
		PathResolutions: make([]LibraryManifestPathResolution, 0),
		Conflicts:       make([]LibraryManifestConflict, 0),
		manifest:        manifest,
		pathMappings:    append([]LibraryManifestPathMapping(nil), options.PathMappings...),
	}

	targetTracks := make(map[string]LibraryManifestTrack, len(target.Tracks))
	for _, track := range target.Tracks {
		if track.ID == "" {
			plan.Conflicts = append(plan.Conflicts, manifestConflict("track", "", "id", "invalid_target", "target contains a track without an id", "requires-review"))
			continue
		}
		if _, exists := targetTracks[track.ID]; exists {
			plan.Conflicts = append(plan.Conflicts, manifestConflict("track", track.ID, "id", "duplicate_target", "target contains duplicate track ids", "requires-review"))
			continue
		}
		targetTracks[track.ID] = track
	}
	for _, track := range manifest.Tracks {
		plan.resolveTrackPaths(track)
		existing, exists := targetTracks[track.ID]
		if !exists {
			plan.TracksToCreate = append(plan.TracksToCreate, track.ID)
			continue
		}
		incomingFingerprint := libraryManifestTrackFingerprint(track)
		existingFingerprint := libraryManifestTrackFingerprint(existing)
		if incomingFingerprint == existingFingerprint {
			plan.TracksToSkip = append(plan.TracksToSkip, track.ID)
			continue
		}
		plan.TracksToUpdate = append(plan.TracksToUpdate, track.ID)
		plan.Conflicts = append(plan.Conflicts, LibraryManifestConflict{
			Entity: "track", EntityID: track.ID, Kind: "metadata_conflict",
			ExistingFingerprint: existingFingerprint, IncomingFingerprint: incomingFingerprint,
			Detail: "incoming track metadata differs from the target", Resolution: resolutionForPolicy(policy),
		})
	}

	targetPlaylists := make(map[string]LibraryManifestPlaylist, len(target.Playlists))
	for _, playlist := range target.Playlists {
		if playlist.ID != "" {
			targetPlaylists[playlist.ID] = playlist
		}
	}
	knownTrackIDs := make(map[string]struct{}, len(targetTracks)+len(manifest.Tracks))
	for id := range targetTracks {
		knownTrackIDs[id] = struct{}{}
	}
	for _, track := range manifest.Tracks {
		knownTrackIDs[track.ID] = struct{}{}
	}
	for _, playlist := range manifest.Playlists {
		for _, trackID := range playlist.TrackIDs {
			if _, exists := knownTrackIDs[trackID]; !exists {
				plan.Conflicts = append(plan.Conflicts, manifestConflict("playlist", playlist.ID, "trackIds", "missing_track", fmt.Sprintf("playlist references unknown track %q", trackID), "requires-review"))
			}
		}
		existing, exists := targetPlaylists[playlist.ID]
		if !exists {
			plan.PlaylistsToCreate = append(plan.PlaylistsToCreate, playlist.ID)
			continue
		}
		incomingFingerprint := libraryManifestPlaylistFingerprint(playlist)
		existingFingerprint := libraryManifestPlaylistFingerprint(existing)
		if incomingFingerprint == existingFingerprint {
			plan.PlaylistsToSkip = append(plan.PlaylistsToSkip, playlist.ID)
			continue
		}
		plan.PlaylistsToUpdate = append(plan.PlaylistsToUpdate, playlist.ID)
		plan.Conflicts = append(plan.Conflicts, LibraryManifestConflict{
			Entity: "playlist", EntityID: playlist.ID, Kind: "playlist_conflict",
			ExistingFingerprint: existingFingerprint, IncomingFingerprint: incomingFingerprint,
			Detail: "incoming playlist membership or metadata differs from the target", Resolution: resolutionForPolicy(policy),
		})
	}

	plan.planRatings(target)
	plan.planFavorites(target)
	plan.planSettings(target)
	plan.planProfileNotes(target)
	return plan, nil
}

func (p *LibraryManifestImportPlan) resolveTrackPaths(track LibraryManifestTrack) {
	refs := []struct {
		field string
		ref   LibraryManifestPathReference
	}{
		{"metadata", track.Paths.Metadata}, {"storageDir", track.Paths.StorageDir}, {"bundle", track.Paths.Bundle},
		{"audio", track.Paths.Audio}, {"timedLyrics", track.Paths.TimedLyrics}, {"video", track.Paths.Video},
		{"lyrics", track.Lyrics.Path}, {"artwork", track.Artwork.Path},
	}
	for _, item := range refs {
		if item.ref.RelativePath == "" && !item.ref.External {
			continue
		}
		resolution := LibraryManifestPathResolution{TrackID: track.ID, Field: item.field, RelativePath: item.ref.RelativePath}
		resolved, err := ResolveLibraryManifestPath(item.ref, p.TargetRoot, p.pathMappings)
		if err != nil {
			resolution.Error = err.Error()
			p.Conflicts = append(p.Conflicts, manifestConflict("track", track.ID, item.field, "path_remap_required", err.Error(), "requires-review"))
		} else {
			resolution.ResolvedPath = resolved
		}
		p.PathResolutions = append(p.PathResolutions, resolution)
	}
}

func (p *LibraryManifestImportPlan) planRatings(target LibraryManifestImportTarget) {
	existing := make(map[string]int, len(target.Ratings))
	for _, rating := range target.Ratings {
		existing[rating.TrackID] = rating.Rating
	}
	for _, rating := range p.manifest.Ratings {
		if current, ok := existing[rating.TrackID]; ok && current != rating.Rating {
			p.Conflicts = append(p.Conflicts, LibraryManifestConflict{
				Entity: "rating", EntityID: rating.TrackID, Kind: "rating_conflict",
				ExistingFingerprint: fingerprintValue(current), IncomingFingerprint: fingerprintValue(rating.Rating),
				Detail: "incoming rating differs from the target", Resolution: resolutionForPolicy(p.ConflictPolicy),
			})
		}
	}
}

func (p *LibraryManifestImportPlan) planFavorites(target LibraryManifestImportTarget) {
	existing := make(map[string]struct{}, len(target.Favorites))
	for _, id := range target.Favorites {
		existing[id] = struct{}{}
	}
	for _, id := range p.manifest.Favorites {
		if _, ok := existing[id]; !ok {
			// Adding a favorite is monotonic and therefore does not overwrite a
			// newer target value.
			continue
		}
	}
}

func (p *LibraryManifestImportPlan) planSettings(target LibraryManifestImportTarget) {
	if target.Settings == (LibraryManifestSettings{}) || p.manifest.Settings == target.Settings {
		return
	}
	p.Conflicts = append(p.Conflicts, LibraryManifestConflict{
		Entity: "settings", EntityID: "default", Kind: "settings_conflict",
		ExistingFingerprint: fingerprintJSON(target.Settings), IncomingFingerprint: fingerprintJSON(p.manifest.Settings),
		Detail: "incoming portable settings differ from the target", Resolution: resolutionForPolicy(p.ConflictPolicy),
	})
}

func (p *LibraryManifestImportPlan) planProfileNotes(target LibraryManifestImportTarget) {
	for key, incoming := range p.manifest.ProfileNotes {
		if existing, ok := target.ProfileNotes[key]; ok && existing != incoming {
			p.Conflicts = append(p.Conflicts, LibraryManifestConflict{
				Entity: "profile-note", EntityID: key, Kind: "profile_note_conflict",
				ExistingFingerprint: fingerprintValue(existing), IncomingFingerprint: fingerprintValue(incoming),
				Detail: "incoming profile note differs from the target", Resolution: resolutionForPolicy(p.ConflictPolicy),
			})
		}
	}
}

// ApplyLibraryManifestImportPlan applies only the detached metadata model. It
// never copies media or lyrics/artwork bytes. The application can later map
// the returned references to its own Store transaction.
func ApplyLibraryManifestImportPlan(plan LibraryManifestImportPlan, target LibraryManifestImportTarget) (LibraryManifestImportTarget, error) {
	if plan.DryRun {
		return cloneLibraryManifestImportTarget(target), nil
	}
	if len(plan.Conflicts) > 0 && plan.ConflictPolicy == LibraryManifestReportConflictsOnly {
		return cloneLibraryManifestImportTarget(target), ErrLibraryManifestConflicts
	}
	result := cloneLibraryManifestImportTarget(target)
	tracks := make(map[string]LibraryManifestTrack, len(result.Tracks))
	for _, track := range result.Tracks {
		tracks[track.ID] = track
	}
	for _, incoming := range plan.manifest.Tracks {
		_, exists := tracks[incoming.ID]
		if exists && plan.ConflictPolicy == LibraryManifestKeepExisting {
			continue
		}
		tracks[incoming.ID] = rewriteManifestTrackPaths(incoming, plan.pathMappings)
	}
	result.Tracks = result.Tracks[:0]
	for _, incoming := range plan.manifest.Tracks {
		if track, exists := tracks[incoming.ID]; exists {
			result.Tracks = append(result.Tracks, track)
		}
	}
	for _, existing := range target.Tracks {
		if _, exists := tracks[existing.ID]; !exists {
			result.Tracks = append(result.Tracks, existing)
		}
	}
	sort.SliceStable(result.Tracks, func(i, j int) bool { return result.Tracks[i].ID < result.Tracks[j].ID })

	playlists := make(map[string]LibraryManifestPlaylist, len(result.Playlists))
	for _, playlist := range result.Playlists {
		playlists[playlist.ID] = playlist
	}
	for _, incoming := range plan.manifest.Playlists {
		if _, exists := playlists[incoming.ID]; exists && plan.ConflictPolicy == LibraryManifestKeepExisting {
			continue
		}
		playlists[incoming.ID] = cloneLibraryManifestPlaylist(incoming)
	}
	result.Playlists = result.Playlists[:0]
	for _, incoming := range plan.manifest.Playlists {
		if playlist, exists := playlists[incoming.ID]; exists {
			result.Playlists = append(result.Playlists, playlist)
		}
	}
	for _, existing := range target.Playlists {
		if _, exists := playlists[existing.ID]; !exists {
			result.Playlists = append(result.Playlists, existing)
		}
	}

	result.Ratings = mergeManifestRatings(plan.manifest.Ratings, target.Ratings, plan.ConflictPolicy)
	result.Favorites = uniqueSortedStrings(append(append([]string(nil), target.Favorites...), plan.manifest.Favorites...))
	result.History = append(append([]LibraryManifestHistoryEntry(nil), target.History...), plan.manifest.History...)
	if plan.ConflictPolicy != LibraryManifestKeepExisting || result.Settings == (LibraryManifestSettings{}) {
		result.Settings = plan.manifest.Settings
	}
	if result.ProfileNotes == nil {
		result.ProfileNotes = make(map[string]string)
	}
	for key, value := range plan.manifest.ProfileNotes {
		if _, exists := result.ProfileNotes[key]; !exists || plan.ConflictPolicy == LibraryManifestPreferIncoming {
			result.ProfileNotes[key] = value
		}
	}
	return result, nil
}

func normalizeLibraryManifest(manifest LibraryManifest) (LibraryManifest, error) {
	if manifest.Format == "" {
		manifest.Format = LibraryManifestFormat
	}
	if manifest.Format != LibraryManifestFormat {
		return LibraryManifest{}, fmt.Errorf("unsupported library manifest format %q", manifest.Format)
	}
	if manifest.SchemaVersion == 0 {
		manifest.SchemaVersion = LibraryManifestSchemaVersion
	}
	if manifest.SchemaVersion > LibraryManifestSchemaVersion {
		return LibraryManifest{}, fmt.Errorf("unsupported library manifest schema version %d", manifest.SchemaVersion)
	}
	if manifest.SchemaVersion < 1 {
		return LibraryManifest{}, fmt.Errorf("invalid library manifest schema version %d", manifest.SchemaVersion)
	}
	if manifest.ExportedAt.IsZero() {
		manifest.ExportedAt = time.Unix(0, 0).UTC()
	}
	manifest.Exclusions = defaultLibraryManifestExclusions()
	if manifest.Tracks == nil {
		manifest.Tracks = []LibraryManifestTrack{}
	}
	if manifest.Playlists == nil {
		manifest.Playlists = []LibraryManifestPlaylist{}
	}
	if manifest.Ratings == nil {
		manifest.Ratings = []LibraryManifestRating{}
	}
	if manifest.History == nil {
		manifest.History = []LibraryManifestHistoryEntry{}
	}
	if manifest.Favorites == nil {
		manifest.Favorites = []string{}
	}
	manifest.Favorites = uniqueSortedStrings(manifest.Favorites)
	for i := range manifest.Tracks {
		if strings.TrimSpace(manifest.Tracks[i].ID) == "" {
			return LibraryManifest{}, errors.New("library manifest contains a track without an id")
		}
		manifest.Tracks[i] = sanitizeManifestTrack(manifest.Tracks[i])
	}
	for i := range manifest.Playlists {
		if strings.TrimSpace(manifest.Playlists[i].ID) == "" {
			return LibraryManifest{}, errors.New("library manifest contains a playlist without an id")
		}
		manifest.Playlists[i].Description = sanitizeManifestText(manifest.Playlists[i].Description)
		manifest.Playlists[i].TrackIDs = append([]string(nil), manifest.Playlists[i].TrackIDs...)
	}
	for i := range manifest.Ratings {
		if manifest.Ratings[i].TrackID == "" || manifest.Ratings[i].Rating < 0 || manifest.Ratings[i].Rating > 5 {
			return LibraryManifest{}, errors.New("library manifest contains an invalid rating")
		}
	}
	for i := range manifest.History {
		manifest.History[i].URL = sanitizeManifestURL(manifest.History[i].URL)
	}
	manifest.Settings.Provider = sanitizeManifestText(manifest.Settings.Provider)
	manifest.Settings.AIModel = sanitizeManifestText(manifest.Settings.AIModel)
	manifest.ProfileNotes = sanitizeManifestMap(manifest.ProfileNotes)
	return manifest, nil
}

func defaultLibraryManifestExclusions() LibraryManifestExclusions {
	return LibraryManifestExclusions{
		APIKeysExcluded: true, SecretsExcluded: true, MediaBinariesExcluded: true,
		LyricsTextExcluded: true, ArtworkBytesExcluded: true,
		ExcludedFields: []string{"apiKeys", "cookiePaths", "authHeaders", "mediaBinaries", "audioBytes", "videoBytes", "lyricsText", "artworkBytes", "artworkDataUrls", "artworkMediaUrls"},
	}
}

func buildLibraryManifestTrack(root string, track TrackRecord) (LibraryManifestTrack, error) {
	pathRef := func(path string) LibraryManifestPathReference { return libraryManifestPathReference(root, path) }
	return LibraryManifestTrack{
		ID: track.ID,
		Metadata: LibraryManifestTrackMetadata{
			Title: track.Title, Artist: track.Artist, Album: track.Album, Genre: track.Genre, Year: track.Year,
			DurationSeconds: cloneIntPointer(track.DurationSeconds), Confidence: track.Confidence,
			MetadataConfidence: track.MetadataConfidence, EnrichmentConfidence: track.EnrichmentConfidence,
			ReleaseType: track.ReleaseType, ISRC: track.ISRC,
			ArtistLinks: sanitizeManifestLinks(track.ArtistLinks), AlbumLinks: sanitizeManifestLinks(track.AlbumLinks), SongLinks: sanitizeManifestLinks(track.SongLinks),
			ArtistTrivia: sanitizeManifestStrings(track.ArtistTrivia), AlbumTrivia: sanitizeManifestStrings(track.AlbumTrivia), SongTrivia: sanitizeManifestStrings(track.SongTrivia),
			SongMeaning: sanitizeManifestText(track.SongMeaning), Tidbits: sanitizeManifestStrings(track.Tidbits), Sources: sanitizeManifestURLs(track.Sources), Notes: sanitizeManifestText(track.MetadataNotes),
		},
		Paths:      LibraryManifestTrackPaths{Metadata: pathRef(track.MetadataPath), StorageDir: pathRef(track.StorageDir), Bundle: pathRef(track.BundlePath), Audio: pathRef(track.AudioPath), TimedLyrics: pathRef(track.LRCPath), Video: pathRef(track.VideoPath)},
		Lyrics:     LibraryManifestLyricsReference{Path: pathRef(track.LyricsPath), Available: track.LyricsIncluded || track.LyricsPath != "", TimedAvailable: track.HasTimedLyrics, Confidence: track.LyricsConfidence, TimedConfidence: track.TimedLyricsConfidence, TimedGranularity: track.TimedLyricsGranularity, Source: sanitizeManifestText(track.LyricsSource), SourceLocation: sanitizeManifestReference(track.LyricsSourceLoc)},
		Artwork:    LibraryManifestArtworkReference{Path: pathRef(track.ArtworkPath), Source: sanitizeManifestURL(track.ArtworkURL), URL: sanitizeManifestURL(track.ArtworkURL)},
		Provenance: LibraryManifestProvenance{SourceKind: track.SourceKind, SourceRef: sanitizeManifestReference(track.SourceRef), SourceURL: sanitizeManifestURL(track.SourceRef), SourceTitle: sanitizeManifestText(track.SourceTitle), SourceChannel: sanitizeManifestText(track.SourceChannel), Sources: sanitizeManifestURLs(track.Sources), MetadataConfidence: track.MetadataConfidence, EnrichmentConfidence: track.EnrichmentConfidence, AIProvider: sanitizeManifestText(track.AIProvider), AIModel: sanitizeManifestText(track.AIModel), AIRan: track.AIRan, GeneratedAt: track.GeneratedAt},
	}, nil
}

func safeLibraryManifestSettings(settings PublicSettings) LibraryManifestSettings {
	return LibraryManifestSettings{Provider: sanitizeManifestText(settings.Provider), AIModel: sanitizeManifestText(settings.AIModel), DownloadMusicVideo: settings.DownloadMusicVideo, KeepOriginalAudio: settings.KeepOriginalAudio, MaxConcurrentDownloads: settings.MaxConcurrentDownloads, MaxConcurrentVideoDownloads: settings.MaxConcurrentVideoDownloads, MaxConcurrentEnrichmentRequests: settings.MaxConcurrentEnrichmentRequests, MaxConcurrentLyricsRequests: settings.MaxConcurrentLyricsRequests, ThrottleOnYTDLPBotErrors: settings.ThrottleOnYTDLPBotErrors}
}

func libraryManifestPathReference(root, path string) LibraryManifestPathReference {
	path = strings.TrimSpace(path)
	if path == "" {
		return LibraryManifestPathReference{}
	}
	if !manifestPathIsAbsolute(path) {
		rel, err := normalizeLibraryManifestRelativePath(path)
		if err == nil {
			return LibraryManifestPathReference{RelativePath: rel}
		}
	}
	if root != "" {
		if rootAbs, err := filepath.Abs(root); err == nil {
			if pathAbs, err := filepath.Abs(path); err == nil {
				if rel, err := filepath.Rel(rootAbs, pathAbs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
					if normalized, err := normalizeLibraryManifestRelativePath(filepath.ToSlash(rel)); err == nil {
						return LibraryManifestPathReference{RelativePath: normalized}
					}
				}
			}
		}
	}
	digest := sha256.Sum256([]byte(path))
	return LibraryManifestPathReference{External: true, PathDigest: hex.EncodeToString(digest[:])}
}

func normalizeLibraryManifestRelativePath(path string) (string, error) {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if path == "" || manifestPathIsAbsolute(path) {
		return "", errors.New("path must be relative")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("path escapes its root")
	}
	return clean, nil
}

func normalizeMappingPrefix(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	return normalizeLibraryManifestRelativePath(path)
}

func manifestPathIsAbsolute(path string) bool {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	return filepath.IsAbs(path) || strings.HasPrefix(path, "/") || regexp.MustCompile(`^[A-Za-z]:/`).MatchString(path)
}

func sanitizeManifestTrack(track LibraryManifestTrack) LibraryManifestTrack {
	track.Paths.Metadata = normalizeManifestPathReference(track.Paths.Metadata)
	track.Paths.StorageDir = normalizeManifestPathReference(track.Paths.StorageDir)
	track.Paths.Bundle = normalizeManifestPathReference(track.Paths.Bundle)
	track.Paths.Audio = normalizeManifestPathReference(track.Paths.Audio)
	track.Paths.TimedLyrics = normalizeManifestPathReference(track.Paths.TimedLyrics)
	track.Paths.Video = normalizeManifestPathReference(track.Paths.Video)
	track.Lyrics.Path = normalizeManifestPathReference(track.Lyrics.Path)
	track.Artwork.Path = normalizeManifestPathReference(track.Artwork.Path)
	track.Metadata = sanitizeManifestMetadata(track.Metadata)
	track.Lyrics.Source = sanitizeManifestText(track.Lyrics.Source)
	track.Lyrics.SourceLocation = sanitizeManifestReference(track.Lyrics.SourceLocation)
	track.Artwork.Source = sanitizeManifestURL(track.Artwork.Source)
	track.Artwork.URL = sanitizeManifestURL(track.Artwork.URL)
	track.Provenance.SourceRef = sanitizeManifestReference(track.Provenance.SourceRef)
	track.Provenance.SourceURL = sanitizeManifestURL(track.Provenance.SourceURL)
	track.Provenance.SourceTitle = sanitizeManifestText(track.Provenance.SourceTitle)
	track.Provenance.SourceChannel = sanitizeManifestText(track.Provenance.SourceChannel)
	track.Provenance.Sources = sanitizeManifestURLs(track.Provenance.Sources)
	return track
}

func normalizeManifestPathReference(reference LibraryManifestPathReference) LibraryManifestPathReference {
	if reference.External {
		reference.RelativePath = ""
		return reference
	}
	if reference.RelativePath == "" {
		return reference
	}
	normalized, err := normalizeLibraryManifestRelativePath(reference.RelativePath)
	if err == nil {
		reference.RelativePath = normalized
		return reference
	}
	digest := sha256.Sum256([]byte(reference.RelativePath))
	return LibraryManifestPathReference{External: true, PathDigest: hex.EncodeToString(digest[:])}
}

func sanitizeManifestMetadata(metadata LibraryManifestTrackMetadata) LibraryManifestTrackMetadata {
	metadata.ArtistLinks = sanitizeManifestLinks(metadata.ArtistLinks)
	metadata.AlbumLinks = sanitizeManifestLinks(metadata.AlbumLinks)
	metadata.SongLinks = sanitizeManifestLinks(metadata.SongLinks)
	metadata.ArtistTrivia = sanitizeManifestStrings(metadata.ArtistTrivia)
	metadata.AlbumTrivia = sanitizeManifestStrings(metadata.AlbumTrivia)
	metadata.SongTrivia = sanitizeManifestStrings(metadata.SongTrivia)
	metadata.SongMeaning = sanitizeManifestText(metadata.SongMeaning)
	metadata.Tidbits = sanitizeManifestStrings(metadata.Tidbits)
	metadata.Sources = sanitizeManifestURLs(metadata.Sources)
	metadata.Notes = sanitizeManifestText(metadata.Notes)
	return metadata
}

func sanitizeManifestLinks(links MetadataLinks) MetadataLinks {
	links.OfficialWebsite = sanitizeManifestURL(links.OfficialWebsite)
	links.Spotify = sanitizeManifestURL(links.Spotify)
	links.AppleMusic = sanitizeManifestURL(links.AppleMusic)
	links.YouTube = sanitizeManifestURL(links.YouTube)
	links.YouTubeMusic = sanitizeManifestURL(links.YouTubeMusic)
	links.Instagram = sanitizeManifestURL(links.Instagram)
	links.X = sanitizeManifestURL(links.X)
	links.Facebook = sanitizeManifestURL(links.Facebook)
	links.Bandcamp = sanitizeManifestURL(links.Bandcamp)
	links.SoundCloud = sanitizeManifestURL(links.SoundCloud)
	links.Wikipedia = sanitizeManifestURL(links.Wikipedia)
	links.MusicBrainz = sanitizeManifestURL(links.MusicBrainz)
	links.Genius = sanitizeManifestURL(links.Genius)
	return links
}

func sanitizeManifestURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lowerValue := strings.ToLower(value)
	if strings.HasPrefix(lowerValue, "data:") || strings.Contains(lowerValue, "base64,") {
		return ""
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return sanitizeManifestText(value)
	}
	if u.User != nil {
		return ""
	}
	query := u.Query()
	for key := range query {
		if manifestSensitiveKey(key) {
			query.Del(key)
		}
	}
	u.RawQuery = query.Encode()
	u.Fragment = ""
	return u.String()
}

func sanitizeManifestReference(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || manifestPathIsAbsolute(value) || strings.Contains(strings.ToLower(value), "data:") {
		return ""
	}
	if strings.Contains(value, "://") {
		return sanitizeManifestURL(value)
	}
	return sanitizeManifestText(value)
}

var manifestSensitiveText = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|refresh[_-]?token|authorization|cookie|password|secret|private[_-]?key|token|signature|sig)\s*[:=]\s*[^\s,;]+`)

func sanitizeManifestText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return manifestSensitiveText.ReplaceAllString(value, "$1=[redacted]")
}

func manifestSensitiveKey(key string) bool {
	switch strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_")) {
	case "api_key", "apikey", "access_token", "refresh_token", "token", "secret", "signature", "sig", "auth", "authorization", "cookie", "password", "private_key":
		return true
	default:
		return false
	}
}

func sanitizeManifestStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if sanitized := sanitizeManifestText(value); sanitized != "" {
			out = append(out, sanitized)
		}
	}
	return out
}

func sanitizeManifestURLs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if sanitized := sanitizeManifestURL(value); sanitized != "" {
			out = append(out, sanitized)
		}
	}
	return out
}

func sanitizeManifestMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[sanitizeManifestText(key)] = sanitizeManifestText(value)
	}
	return out
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func manifestConflict(entity, id, field, kind, detail, resolution string) LibraryManifestConflict {
	return LibraryManifestConflict{Entity: entity, EntityID: id, Field: field, Kind: kind, Detail: detail, Resolution: resolution}
}
func resolutionForPolicy(policy LibraryManifestConflictPolicy) string {
	if policy == LibraryManifestPreferIncoming {
		return "prefer-incoming"
	}
	if policy == LibraryManifestKeepExisting {
		return "keep-existing"
	}
	return "requires-user-choice"
}

func fingerprintValue(value any) string { return fingerprintJSON(value) }
func fingerprintJSON(value any) string {
	data, _ := json.Marshal(value)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func libraryManifestTrackFingerprint(track LibraryManifestTrack) string {
	return fingerprintJSON(struct {
		ID         string
		Metadata   LibraryManifestTrackMetadata
		Lyrics     LibraryManifestLyricsReference
		Artwork    LibraryManifestArtworkReference
		Provenance LibraryManifestProvenance
	}{track.ID, track.Metadata, track.Lyrics, track.Artwork, track.Provenance})
}
func libraryManifestPlaylistFingerprint(playlist LibraryManifestPlaylist) string {
	return fingerprintJSON(playlist)
}

func cloneLibraryManifestPlaylist(playlist LibraryManifestPlaylist) LibraryManifestPlaylist {
	playlist.TrackIDs = append([]string(nil), playlist.TrackIDs...)
	return playlist
}

func rewriteManifestTrackPaths(track LibraryManifestTrack, mappings []LibraryManifestPathMapping) LibraryManifestTrack {
	rewrite := func(reference LibraryManifestPathReference) LibraryManifestPathReference {
		if reference.RelativePath == "" || reference.External {
			return reference
		}
		mapped, err := RemapLibraryManifestRelativePath(reference.RelativePath, mappings)
		if err == nil {
			reference.RelativePath = mapped
		}
		return reference
	}
	track.Paths.Metadata = rewrite(track.Paths.Metadata)
	track.Paths.StorageDir = rewrite(track.Paths.StorageDir)
	track.Paths.Bundle = rewrite(track.Paths.Bundle)
	track.Paths.Audio = rewrite(track.Paths.Audio)
	track.Paths.TimedLyrics = rewrite(track.Paths.TimedLyrics)
	track.Paths.Video = rewrite(track.Paths.Video)
	track.Lyrics.Path = rewrite(track.Lyrics.Path)
	track.Artwork.Path = rewrite(track.Artwork.Path)
	return track
}

func mergeManifestRatings(incoming, existing []LibraryManifestRating, policy LibraryManifestConflictPolicy) []LibraryManifestRating {
	values := make(map[string]int, len(existing))
	for _, item := range existing {
		values[item.TrackID] = item.Rating
	}
	for _, item := range incoming {
		if _, exists := values[item.TrackID]; !exists || policy == LibraryManifestPreferIncoming {
			values[item.TrackID] = item.Rating
		}
	}
	out := make([]LibraryManifestRating, 0, len(values))
	for id, rating := range values {
		out = append(out, LibraryManifestRating{TrackID: id, Rating: rating})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TrackID < out[j].TrackID })
	return out
}

func cloneLibraryManifestImportTarget(target LibraryManifestImportTarget) LibraryManifestImportTarget {
	result := target
	result.Tracks = append([]LibraryManifestTrack(nil), target.Tracks...)
	for i := range result.Tracks {
		result.Tracks[i].Metadata.ArtistTrivia = append([]string(nil), result.Tracks[i].Metadata.ArtistTrivia...)
		result.Tracks[i].Metadata.AlbumTrivia = append([]string(nil), result.Tracks[i].Metadata.AlbumTrivia...)
		result.Tracks[i].Metadata.SongTrivia = append([]string(nil), result.Tracks[i].Metadata.SongTrivia...)
		result.Tracks[i].Metadata.Tidbits = append([]string(nil), result.Tracks[i].Metadata.Tidbits...)
		result.Tracks[i].Metadata.Sources = append([]string(nil), result.Tracks[i].Metadata.Sources...)
		result.Tracks[i].Provenance.Sources = append([]string(nil), result.Tracks[i].Provenance.Sources...)
	}
	result.Playlists = append([]LibraryManifestPlaylist(nil), target.Playlists...)
	for i := range result.Playlists {
		result.Playlists[i] = cloneLibraryManifestPlaylist(result.Playlists[i])
	}
	result.Ratings = append([]LibraryManifestRating(nil), target.Ratings...)
	result.Favorites = append([]string(nil), target.Favorites...)
	result.History = append([]LibraryManifestHistoryEntry(nil), target.History...)
	result.ProfileNotes = sanitizeManifestMap(target.ProfileNotes)
	return result
}

func (p LibraryManifestImportPlan) MarshalJSON() ([]byte, error) {
	type planJSON LibraryManifestImportPlan
	return json.Marshal(planJSON(p))
}

// Keep io imported in this file's public API surface for callers that want a
// stream-oriented adapter without coupling the manifest format to a file.
func DecodeLibraryManifest(r io.Reader) (LibraryManifest, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return LibraryManifest{}, err
	}
	return UnmarshalLibraryManifest(data)
}
