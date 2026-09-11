package songstore

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var invalidSegment = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]+`)
var unsafeTrim = regexp.MustCompile(`[. ]+$`)
var multipleSpace = regexp.MustCompile(`\s+`)

var windowsReservedNames = map[string]struct{}{
	"con":  {},
	"prn":  {},
	"aux":  {},
	"nul":  {},
	"com1": {},
	"com2": {},
	"com3": {},
	"com4": {},
	"com5": {},
	"com6": {},
	"com7": {},
	"com8": {},
	"com9": {},
	"lpt1": {},
	"lpt2": {},
	"lpt3": {},
	"lpt4": {},
	"lpt5": {},
	"lpt6": {},
	"lpt7": {},
	"lpt8": {},
	"lpt9": {},
}

func normalizeAudioFormat(value string) string {
	value = strings.TrimSpace(strings.ToLower(strings.TrimPrefix(value, ".")))
	switch value {
	case "mp3", "wav", "flac", "ogg":
		return value
	case "oga":
		return "ogg"
	case "wave":
		return "wav"
	case "fla":
		return "flac"
	case "m4a", "aac", "opus", "webm":
		return value
	default:
		return ""
	}
}

type SongStoragePlan struct {
	ArtistDir    string
	AlbumDir     string
	BaseName     string
	AudioPath    string
	LyricsPath   string
	LRCPath      string
	ArtworkPath  string
	VideoPath    string
	MetadataPath string
}

type SongMetadata struct {
	Version              int             `json:"version,omitempty"`
	TrackID              string          `json:"track_id,omitempty"`
	Title                string          `json:"title"`
	Artist               string          `json:"artist"`
	Album                string          `json:"album"`
	TrackNumber          *int            `json:"track_number,omitempty"`
	Year                 *int            `json:"year,omitempty"`
	Genre                string          `json:"genre"`
	DurationSeconds      *int            `json:"duration_seconds,omitempty"`
	Confidence           string          `json:"confidence,omitempty"`
	MetadataConfidence   string          `json:"metadata_confidence,omitempty"`
	EnrichmentConfidence string          `json:"enrichment_confidence,omitempty"`
	ReleaseType          string          `json:"release_type,omitempty"`
	ISRC                 string          `json:"isrc,omitempty"`
	ArtistLinks          MetadataLinks   `json:"artist_links,omitempty"`
	AlbumLinks           MetadataLinks   `json:"album_links,omitempty"`
	SongLinks            MetadataLinks   `json:"song_links,omitempty"`
	ArtistTrivia         []string        `json:"artist_trivia,omitempty"`
	AlbumTrivia          []string        `json:"album_trivia,omitempty"`
	SongTrivia           []string        `json:"song_trivia,omitempty"`
	SongMeaning          string          `json:"song_meaning,omitempty"`
	Tidbits              []string        `json:"tidbits,omitempty"`
	Sources              []string        `json:"sources,omitempty"`
	MetadataNotes        string          `json:"metadata_notes,omitempty"`
	Source               SongSource      `json:"source"`
	Audio                SongAudio       `json:"audio"`
	Lyrics               SongLyrics      `json:"lyrics"`
	TimedLyrics          SongTimedLyrics `json:"timed_lyrics"`
	Artwork              SongArtwork     `json:"artwork"`
	Video                SongVideo       `json:"video"`
	AI                   SongAI          `json:"ai"`
	StorageDir           string          `json:"storage_dir,omitempty"`
	MetadataPath         string          `json:"metadata_path,omitempty"`
}

type SongSource struct {
	URL          string    `json:"url"`
	Provider     string    `json:"provider"`
	VideoID      string    `json:"video_id"`
	VideoTitle   string    `json:"video_title"`
	Channel      string    `json:"channel"`
	DownloadedAt time.Time `json:"downloaded_at"`
}

type SongAudio struct {
	Filename         string `json:"filename"`
	Format           string `json:"format"`
	Codec            string `json:"codec,omitempty"`
	SourceFormat     string `json:"source_format,omitempty"`
	OriginalFilename string `json:"original_filename,omitempty"`
	OriginalFormat   string `json:"original_format,omitempty"`
	OriginalPath     string `json:"original_path,omitempty"`
	Path             string `json:"path"`
	Bitrate          *int   `json:"bitrate,omitempty"`
	SampleRate       *int   `json:"sample_rate,omitempty"`
	Channels         *int   `json:"channels,omitempty"`
}

type SongLyrics struct {
	Filename   string `json:"filename"`
	Path       string `json:"path"`
	HasLyrics  bool   `json:"has_lyrics"`
	IsComplete bool   `json:"is_complete"`
	Confidence string `json:"confidence"`
	Source     string `json:"source"`
	SourceLoc  string `json:"source_loc,omitempty"`
	Notes      string `json:"notes"`
}

type SongTimedLyrics struct {
	Filename       string `json:"filename"`
	Path           string `json:"path"`
	HasTimedLyrics bool   `json:"has_timed_lyrics"`
	Format         string `json:"format"`
	Granularity    string `json:"granularity"`
	Confidence     string `json:"confidence"`
	Source         string `json:"source"`
}

type SongArtwork struct {
	Filename       string    `json:"filename"`
	Path           string    `json:"path"`
	Source         string    `json:"source,omitempty"`
	ReleaseID      string    `json:"release_id,omitempty"`
	ReleaseGroupID string    `json:"release_group_id,omitempty"`
	URL            string    `json:"url,omitempty"`
	FetchedAt      time.Time `json:"fetched_at,omitempty"`
}

type SongVideo struct {
	Filename  string    `json:"filename"`
	Path      string    `json:"path"`
	Source    string    `json:"source,omitempty"`
	URL       string    `json:"url,omitempty"`
	FetchedAt time.Time `json:"fetched_at,omitempty"`
}

type SongAI struct {
	Provider    string        `json:"provider"`
	Model       string        `json:"model"`
	Ran         bool          `json:"ran,omitempty"`
	GeneratedAt time.Time     `json:"generated_at"`
	Status      string        `json:"status,omitempty"`
	Message     string        `json:"message,omitempty"`
	Context     SongAIContext `json:"context,omitempty"`
}

type MetadataLinks struct {
	OfficialWebsite string `json:"official_website,omitempty"`
	Spotify         string `json:"spotify,omitempty"`
	AppleMusic      string `json:"apple_music,omitempty"`
	YouTube         string `json:"youtube,omitempty"`
	YouTubeMusic    string `json:"youtube_music,omitempty"`
	Instagram       string `json:"instagram,omitempty"`
	X               string `json:"x,omitempty"`
	Facebook        string `json:"facebook,omitempty"`
	Bandcamp        string `json:"bandcamp,omitempty"`
	SoundCloud      string `json:"soundcloud,omitempty"`
	Wikipedia       string `json:"wikipedia,omitempty"`
	MusicBrainz     string `json:"musicbrainz,omitempty"`
	Genius          string `json:"genius,omitempty"`
}

type SongAIContext struct {
	FileName                 string `json:"file_name,omitempty"`
	SourceRef                string `json:"source_ref,omitempty"`
	SourceURL                string `json:"source_url,omitempty"`
	SourceKind               string `json:"source_kind,omitempty"`
	SourceTitle              string `json:"source_title,omitempty"`
	SourceUploader           string `json:"source_uploader,omitempty"`
	SourceChannel            string `json:"source_channel,omitempty"`
	LibraryRoot              string `json:"library_root,omitempty"`
	SourceDescriptionHint    string `json:"source_description_hint,omitempty"`
	DurationSeconds          *int   `json:"duration_seconds,omitempty"`
	DurationHint             string `json:"duration_hint,omitempty"`
	HasTranscript            bool   `json:"has_transcript,omitempty"`
	HasTimestampedTranscript bool   `json:"has_timestamped_transcript,omitempty"`
	InputText                string `json:"input_text,omitempty"`
	UserContext              string `json:"user_context,omitempty"`
	ArtistHint               string `json:"artist_hint,omitempty"`
	AlbumHint                string `json:"album_hint,omitempty"`
	TitleHint                string `json:"title_hint,omitempty"`
	YearHint                 *int   `json:"year_hint,omitempty"`
	GenreHint                string `json:"genre_hint,omitempty"`
	TrackNumberHint          *int   `json:"track_number_hint,omitempty"`
	Notes                    string `json:"notes,omitempty"`
	ResolvedTitle            string `json:"resolved_title,omitempty"`
	ResolvedArtist           string `json:"resolved_artist,omitempty"`
	ResolvedAlbum            string `json:"resolved_album,omitempty"`
	ResolvedTrackNumber      string `json:"resolved_track_number,omitempty"`
	ResolvedYear             string `json:"resolved_year,omitempty"`
	ResolvedGenre            string `json:"resolved_genre,omitempty"`
	ResolvedConfidence       string `json:"resolved_confidence,omitempty"`
	ResolvedNotes            string `json:"resolved_notes,omitempty"`
	TargetArtistDir          string `json:"target_artist_dir,omitempty"`
	TargetAlbumDir           string `json:"target_album_dir,omitempty"`
	TargetBaseName           string `json:"target_base_name,omitempty"`
	TargetAudioPath          string `json:"target_audio_path,omitempty"`
	TargetLyricsPath         string `json:"target_lyrics_path,omitempty"`
	TargetTimedLyricsPath    string `json:"target_timed_lyrics_path,omitempty"`
	TargetMetadataPath       string `json:"target_metadata_path,omitempty"`
}

type TimedLyricLine struct {
	StartSeconds float64  `json:"start"`
	EndSeconds   *float64 `json:"end,omitempty"`
	Text         string   `json:"text"`
	Label        string   `json:"label,omitempty"`
}

type LyricsResult struct {
	Text              string           `json:"lyrics"`
	TimedLyrics       []TimedLyricLine `json:"timed_lyrics"`
	TimingGranularity string           `json:"timing_granularity"`
	Confidence        string           `json:"confidence"`
	Source            string           `json:"source"`
	SourceLoc         string           `json:"source_loc,omitempty"`
	IsComplete        bool             `json:"is_complete"`
	Notes             string           `json:"notes"`
}

type MetadataResult struct {
	Title                string        `json:"title"`
	Artist               string        `json:"artist"`
	Album                string        `json:"album"`
	TrackNumber          *int          `json:"track_number,omitempty"`
	Year                 *int          `json:"year,omitempty"`
	Genre                string        `json:"genre"`
	DurationSeconds      *int          `json:"duration_seconds,omitempty"`
	Confidence           string        `json:"confidence"`
	MetadataConfidence   string        `json:"metadata_confidence,omitempty"`
	EnrichmentConfidence string        `json:"enrichment_confidence,omitempty"`
	ReleaseType          string        `json:"release_type,omitempty"`
	ISRC                 string        `json:"isrc,omitempty"`
	ArtistLinks          MetadataLinks `json:"artist_links,omitempty"`
	AlbumLinks           MetadataLinks `json:"album_links,omitempty"`
	SongLinks            MetadataLinks `json:"song_links,omitempty"`
	ArtistTrivia         []string      `json:"artist_trivia,omitempty"`
	AlbumTrivia          []string      `json:"album_trivia,omitempty"`
	SongTrivia           []string      `json:"song_trivia,omitempty"`
	SongMeaning          string        `json:"song_meaning,omitempty"`
	Tidbits              []string      `json:"tidbits,omitempty"`
	Sources              []string      `json:"sources,omitempty"`
	Notes                string        `json:"notes"`
}

func BuildSongStoragePlan(root string, metadata SongMetadata) (SongStoragePlan, error) {
	return buildSongStoragePlan(root, metadata, nil)
}

func BuildSongStoragePlanForReprocess(root string, metadata SongMetadata, reservedPaths ...string) (SongStoragePlan, error) {
	ignore := make(map[string]struct{}, len(reservedPaths))
	for _, path := range reservedPaths {
		if trimmed := strings.TrimSpace(path); trimmed != "" {
			ignore[filepath.Clean(trimmed)] = struct{}{}
		}
	}
	return buildSongStoragePlan(root, metadata, ignore)
}

func buildSongStoragePlan(root string, metadata SongMetadata, ignore map[string]struct{}) (SongStoragePlan, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return SongStoragePlan{}, err
	}
	artistName := SanitizePathSegment(metadata.Artist, "Unknown Artist")
	albumName := SanitizePathSegment(metadata.Album, "Unknown Album")
	audioFormat := normalizeAudioFormat(metadata.Audio.Format)
	if audioFormat == "" {
		audioFormat = "mp3"
	}
	artistDir := filepath.Join(root, artistName)
	albumDir := filepath.Join(artistDir, albumName)
	baseName := BuildSongBaseName(metadata)
	baseName = uniqueBaseName(albumDir, baseName, ignore)
	return SongStoragePlan{
		ArtistDir:    artistDir,
		AlbumDir:     albumDir,
		BaseName:     baseName,
		AudioPath:    filepath.Join(albumDir, baseName+"."+audioFormat),
		LyricsPath:   filepath.Join(albumDir, baseName+".txt"),
		LRCPath:      filepath.Join(albumDir, baseName+".lrc"),
		ArtworkPath:  filepath.Join(albumDir, "cover.jpg"),
		VideoPath:    filepath.Join(albumDir, baseName+".mp4"),
		MetadataPath: filepath.Join(albumDir, baseName+".metadata.json"),
	}, nil
}

func BuildSongBaseName(metadata SongMetadata) string {
	title := SanitizePathSegment(metadata.Title, "Untitled Song")
	if metadata.TrackNumber != nil && *metadata.TrackNumber > 0 {
		return fmt.Sprintf("%02d - %s", *metadata.TrackNumber, title)
	}
	return title
}

func SanitizePathSegment(value string, fallback string) string {
	value = strings.TrimSpace(value)
	value = invalidSegment.ReplaceAllString(value, "-")
	value = multipleSpace.ReplaceAllString(value, " ")
	value = unsafeTrim.ReplaceAllString(value, "")
	value = strings.TrimSpace(value)
	value = strings.Trim(value, ". ")
	if value == "" {
		value = fallback
	}
	value = unsafeTrim.ReplaceAllString(value, "")
	value = strings.TrimSpace(value)
	value = strings.Trim(value, ". ")
	if value == "" {
		value = fallback
	}
	base := strings.ToLower(value)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if _, ok := windowsReservedNames[base]; ok {
		value = value + "_"
	}
	value = strings.Trim(value, ". ")
	if value == "" {
		value = fallback
	}
	if _, ok := windowsReservedNames[strings.ToLower(strings.TrimSuffix(value, filepath.Ext(value)))]; ok {
		value = fallback + "_"
	}
	return value
}

func WriteSongFiles(plan SongStoragePlan, audioSourcePath string, originalAudioSourcePath string, lyrics LyricsResult, metadata SongMetadata) error {
	if err := os.MkdirAll(plan.AlbumDir, 0o755); err != nil {
		return err
	}

	metadata = enrichMetadataWithPaths(metadata, plan, lyrics)
	if metadata.TrackID == "" {
		metadata.TrackID = TrackIDFromPath(plan.MetadataPath)
	}

	targets := []string{plan.AudioPath, plan.LyricsPath, plan.LRCPath, plan.MetadataPath}
	if strings.TrimSpace(metadata.Audio.OriginalPath) != "" && strings.TrimSpace(originalAudioSourcePath) != "" {
		targets = append(targets, metadata.Audio.OriginalPath)
	}
	backups, err := snapshotAtomicTargets(targets)
	if err != nil {
		return err
	}
	rollback := func(writeErr error) error {
		if rollbackErr := backups.rollback(); rollbackErr != nil {
			return fmt.Errorf("%w (rollback failed: %v)", writeErr, rollbackErr)
		}
		return writeErr
	}

	if err := copyFileAtomic(audioSourcePath, plan.AudioPath); err != nil {
		return rollback(err)
	}

	if strings.TrimSpace(metadata.Audio.OriginalPath) != "" && strings.TrimSpace(originalAudioSourcePath) != "" {
		if err := copyFileAtomic(originalAudioSourcePath, metadata.Audio.OriginalPath); err != nil {
			return rollback(err)
		}
	}

	if err := WritePlainLyrics(plan.LyricsPath, lyrics.Text); err != nil {
		return rollback(err)
	}

	lrcLines := lyrics.TimedLyrics
	granularity := normalizeGranularity(lyrics.TimingGranularity, lyrics.TimedLyrics)
	if !shouldWriteLRC(granularity, lrcLines) {
		lrcLines = nil
	}
	if err := WriteLRC(plan.LRCPath, lrcLines); err != nil {
		return rollback(err)
	}

	if err := WriteMetadataJSON(plan.MetadataPath, metadata); err != nil {
		return rollback(err)
	}
	backups.commit()

	return nil
}

func shouldWriteLRC(granularity string, timedLyrics []TimedLyricLine) bool {
	return strings.EqualFold(granularity, "line") && len(timedLyrics) > 0
}

func enrichMetadataWithPaths(metadata SongMetadata, plan SongStoragePlan, lyrics LyricsResult) SongMetadata {
	metadata.StorageDir = plan.AlbumDir
	metadata.MetadataPath = plan.MetadataPath
	metadata.Audio.Format = normalizeAudioFormat(metadata.Audio.Format)
	if metadata.Audio.Format == "" {
		metadata.Audio.Format = "mp3"
	}
	metadata.Audio.SourceFormat = normalizeAudioFormat(metadata.Audio.SourceFormat)
	if metadata.Audio.SourceFormat == "" {
		metadata.Audio.SourceFormat = metadata.Audio.Format
	}
	metadata.Audio = SongAudio{
		Filename:         filepath.Base(plan.AudioPath),
		Format:           metadata.Audio.Format,
		Codec:            metadata.Audio.Codec,
		SourceFormat:     metadata.Audio.SourceFormat,
		OriginalFilename: metadata.Audio.OriginalFilename,
		OriginalFormat:   metadata.Audio.OriginalFormat,
		OriginalPath:     metadata.Audio.OriginalPath,
		Path:             plan.AudioPath,
		Bitrate:          metadata.Audio.Bitrate,
		SampleRate:       metadata.Audio.SampleRate,
		Channels:         metadata.Audio.Channels,
	}
	metadata.Lyrics = SongLyrics{
		Filename:   filepath.Base(plan.LyricsPath),
		Path:       plan.LyricsPath,
		HasLyrics:  strings.TrimSpace(lyrics.Text) != "",
		IsComplete: lyrics.IsComplete,
		Confidence: lyrics.Confidence,
		Source:     lyrics.Source,
		SourceLoc:  lyrics.SourceLoc,
		Notes:      lyrics.Notes,
	}
	metadata.TimedLyrics = SongTimedLyrics{
		Filename:       filepath.Base(plan.LRCPath),
		Path:           plan.LRCPath,
		HasTimedLyrics: len(lyrics.TimedLyrics) > 0,
		Format:         "lrc",
		Granularity:    normalizeGranularity(lyrics.TimingGranularity, lyrics.TimedLyrics),
		Confidence:     lyrics.Confidence,
		Source:         lyrics.Source,
	}
	if metadata.Artwork.Path != "" {
		if metadata.Artwork.Filename == "" {
			metadata.Artwork.Filename = filepath.Base(metadata.Artwork.Path)
		}
	} else if metadata.Artwork.Source != "" || metadata.Artwork.URL != "" || metadata.Artwork.ReleaseID != "" || metadata.Artwork.ReleaseGroupID != "" {
		metadata.Artwork.Filename = filepath.Base(plan.ArtworkPath)
		metadata.Artwork.Path = plan.ArtworkPath
	}
	if metadata.Video.Path != "" {
		if metadata.Video.Filename == "" {
			metadata.Video.Filename = filepath.Base(metadata.Video.Path)
		}
	} else if metadata.Video.Source != "" || metadata.Video.URL != "" {
		metadata.Video.Filename = filepath.Base(plan.VideoPath)
		metadata.Video.Path = plan.VideoPath
	}
	if metadata.AI.GeneratedAt.IsZero() {
		metadata.AI.GeneratedAt = time.Now().UTC()
	}
	if metadata.AI.Provider == "" {
		metadata.AI.Provider = "unknown"
	}
	return metadata
}

func normalizeGranularity(granularity string, timedLyrics []TimedLyricLine) string {
	granularity = strings.ToLower(strings.TrimSpace(granularity))
	switch granularity {
	case "line", "block":
		return granularity
	default:
		if len(timedLyrics) > 0 {
			return "line"
		}
		return "none"
	}
}

func WritePlainLyrics(path string, lyrics string) error {
	return writeAtomicFile(path, []byte(lyrics), 0o644)
}

func WriteLRC(path string, timedLyrics []TimedLyricLine) error {
	if len(timedLyrics) == 0 {
		return writeAtomicFile(path, nil, 0o644)
	}
	var b strings.Builder
	for _, line := range timedLyrics {
		if line.Text == "" && line.StartSeconds == 0 {
			continue
		}
		b.WriteString(formatLRCStamp(line.StartSeconds))
		b.WriteString(line.Text)
		b.WriteByte('\n')
	}
	return writeAtomicFile(path, []byte(b.String()), 0o644)
}

func formatLRCStamp(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	minutes := int(seconds) / 60
	secs := int(seconds) % 60
	centis := int(math.Round((seconds - float64(int(seconds))) * 100))
	if centis == 100 {
		centis = 0
		secs++
	}
	if secs == 60 {
		secs = 0
		minutes++
	}
	return fmt.Sprintf("[%02d:%02d.%02d]", minutes, secs, centis)
}

func WriteMetadataJSON(path string, metadata SongMetadata) error {
	if metadata.Version == 0 {
		metadata.Version = songMetadataSchemaVersion
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicFile(path, data, 0o644)
}

func WriteBinaryFile(path string, data []byte) error {
	return writeAtomicFile(path, data, 0o644)
}

func LoadSongMetadata(path string) (SongMetadata, error) {
	info, err := os.Stat(path)
	if err != nil {
		return SongMetadata{}, err
	}
	if info.IsDir() {
		return loadMetadataFromDirectory(path)
	}
	ext := strings.ToLower(filepath.Ext(path))
	if strings.HasSuffix(strings.ToLower(path), ".metadata.json") {
		return ReadMetadataJSON(path)
	}
	if ext == ".mldx" {
		return ReadLegacyBundle(path)
	}
	if ext == ".json" {
		return ReadMetadataJSON(path)
	}
	metadataPath := siblingMetadataPath(path)
	if metadataPath != "" {
		if _, err := os.Stat(metadataPath); err == nil {
			return ReadMetadataJSON(metadataPath)
		}
	}
	return SongMetadata{}, fmt.Errorf("no metadata found for %s", path)
}

func ReadMetadataJSON(path string) (SongMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SongMetadata{}, err
	}
	var metadata SongMetadata
	if err := decodeSongMetadataJSON(data, &metadata); err != nil {
		return SongMetadata{}, err
	}
	if err := validateSongMetadataVersion(metadata.Version); err != nil {
		return SongMetadata{}, err
	}
	if metadata.Version == 0 {
		metadata.Version = songMetadataSchemaVersion
	}
	if metadata.TrackID == "" {
		metadata.TrackID = TrackIDFromPath(path)
	}
	metadata, _ = EnsureAIStatus(metadata)
	return metadata, nil
}

func EnsureAIStatus(metadata SongMetadata) (SongMetadata, bool) {
	if strings.TrimSpace(metadata.AI.Status) != "" {
		return metadata, false
	}
	if strings.TrimSpace(metadata.AI.Provider) == "" && strings.TrimSpace(metadata.AI.Model) == "" && metadata.AI.GeneratedAt.IsZero() {
		return metadata, false
	}
	notes := strings.ToLower(metadata.MetadataNotes)
	if strings.Contains(notes, "fallback metadata derived locally") || strings.Contains(notes, "ai enrichment was unavailable") {
		metadata.AI.Status = "failed"
		metadata.AI.Message = "Missing API key"
		metadata.AI.Ran = false
		return metadata, true
	}
	if metadata.Lyrics.HasLyrics || metadata.TimedLyrics.HasTimedLyrics {
		metadata.AI.Status = "success"
		metadata.AI.Message = "Metadata and lyrics generated."
		metadata.AI.Ran = true
		return metadata, true
	}
	metadata.AI.Status = "partial"
	metadata.AI.Message = "Metadata generated; lyrics unavailable."
	metadata.AI.Ran = true
	return metadata, true
}

func ReadLegacyBundle(path string) (SongMetadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return SongMetadata{}, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return SongMetadata{}, err
	}
	reader, err := zip.NewReader(file, stat.Size())
	if err != nil {
		return SongMetadata{}, err
	}
	for _, entry := range reader.File {
		if entry.Name != "manifest.json" {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			return SongMetadata{}, err
		}
		defer rc.Close()
		var legacy struct {
			Artist        string    `json:"artist"`
			Album         string    `json:"album"`
			Title         string    `json:"title"`
			Genre         string    `json:"genre"`
			Year          string    `json:"year"`
			Lyrics        string    `json:"lyrics"`
			SourceKind    string    `json:"sourceKind"`
			SourceRef     string    `json:"sourceRef"`
			OriginalName  string    `json:"originalName"`
			AudioEntry    string    `json:"audioEntry"`
			MetadataEntry string    `json:"metadataEntry"`
			LyricsEntry   string    `json:"lyricsEntry"`
			Hash          string    `json:"hash"`
			CreatedAt     time.Time `json:"createdAt"`
		}
		if err := json.NewDecoder(rc).Decode(&legacy); err != nil {
			return SongMetadata{}, err
		}
		var year *int
		if parsedYear, err := strconv.Atoi(strings.TrimSpace(legacy.Year)); err == nil {
			year = &parsedYear
		}
		return SongMetadata{
			Version: 1,
			TrackID: TrackIDFromPath(path),
			Title:   legacy.Title,
			Artist:  legacy.Artist,
			Album:   legacy.Album,
			Year:    year,
			Genre:   legacy.Genre,
			Source: SongSource{
				URL:          legacy.SourceRef,
				Provider:     legacy.SourceKind,
				VideoTitle:   legacy.OriginalName,
				DownloadedAt: legacy.CreatedAt,
			},
			Audio: SongAudio{
				Filename: legacy.AudioEntry,
				Format:   strings.TrimPrefix(filepath.Ext(legacy.AudioEntry), "."),
				Path:     path,
			},
			Lyrics: SongLyrics{
				Filename:   legacy.LyricsEntry,
				Path:       path,
				HasLyrics:  strings.TrimSpace(legacy.Lyrics) != "",
				IsComplete: strings.TrimSpace(legacy.Lyrics) != "",
				Confidence: "medium",
				Source:     legacy.SourceKind,
				Notes:      "Loaded from legacy bundled format.",
			},
			TimedLyrics: SongTimedLyrics{
				Filename:       legacy.LyricsEntry,
				Path:           path,
				HasTimedLyrics: false,
				Format:         "lrc",
				Granularity:    "none",
				Confidence:     "low",
				Source:         "none",
			},
			AI: SongAI{
				Provider:    "legacy",
				Model:       "",
				GeneratedAt: legacy.CreatedAt,
			},
			StorageDir:   filepath.Dir(path),
			MetadataPath: path,
		}, nil
	}
	return SongMetadata{}, fmt.Errorf("manifest.json not found in legacy bundle")
}

func TrackIDFromPath(path string) string {
	sum := sha1.Sum([]byte(strings.ToLower(filepath.Clean(path))))
	return hex.EncodeToString(sum[:8])
}

func loadMetadataFromDirectory(dir string) (SongMetadata, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return SongMetadata{}, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(strings.ToLower(name), ".metadata.json") {
			return ReadMetadataJSON(filepath.Join(dir, name))
		}
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".mldx") {
			return ReadLegacyBundle(filepath.Join(dir, entry.Name()))
		}
	}
	return SongMetadata{}, fmt.Errorf("no metadata file found in %s", dir)
}

func siblingMetadataPath(path string) string {
	if strings.HasSuffix(strings.ToLower(path), ".metadata.json") {
		return path
	}
	if strings.EqualFold(filepath.Ext(path), ".mldx") {
		return ""
	}
	return strings.TrimSuffix(path, filepath.Ext(path)) + ".metadata.json"
}

func uniqueBaseName(dir, base string, ignore map[string]struct{}) string {
	candidate := base
	for i := 1; ; i++ {
		if !anyFileExists(dir, candidate, ignore) {
			return candidate
		}
		candidate = fmt.Sprintf("%s (%d)", base, i+1)
	}
}

func anyFileExists(dir, base string, ignore map[string]struct{}) bool {
	paths := []string{
		filepath.Join(dir, base+".mp3"),
		filepath.Join(dir, base+".txt"),
		filepath.Join(dir, base+".lrc"),
		filepath.Join(dir, base+".metadata.json"),
	}
	for _, path := range paths {
		if ignore != nil {
			if _, ok := ignore[filepath.Clean(path)]; ok {
				continue
			}
		}
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func copyFileAtomic(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	return writeAtomicStream(dst, in, 0o644)
}

func writeAtomicFile(path string, data []byte, perm os.FileMode) error {
	return writeAtomicStream(path, bytes.NewReader(data), perm)
}

func writeAtomicStream(path string, r io.Reader, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if err := atomicWriteFailure(atomicWriteStageWrite, path); err != nil {
		cleanup()
		return err
	}
	if _, err := io.Copy(tmp, r); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := atomicWriteFailure(atomicWriteStageRename, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
