package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"melodex/internal/songstore"
)

type aiClient struct {
	baseURL string
	apiKey  string
	model   string
}

func newAIClient(settings storedSettings) *aiClient {
	baseURL := strings.TrimSpace(settings.AIBaseURL)
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := strings.TrimSpace(settings.AIModel)
	if model == "" {
		model = "gpt-4.1-mini"
	}
	return &aiClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  strings.TrimSpace(settings.APIKey),
		model:   model,
	}
}

func (c *aiClient) available() bool {
	return c.apiKey != ""
}

func (c *aiClient) enrichMetadata(ctx context.Context, prompts *PromptLibrary, input EnrichmentInput) (songstore.MetadataResult, error) {
	systemPrompt, userPrompt, err := prompts.MetadataPrompt(input.PromptMap())
	log.Printf("prompts lookup started for %s", input.FileName)
	if err != nil {
		return songstore.MetadataResult{}, err
	}
	var result songstore.MetadataResult
	if err := c.postJSON(ctx, systemPrompt, userPrompt, &result); err != nil {
		return songstore.MetadataResult{}, err
	}
	return result, nil
}

func (c *aiClient) enrichLyrics(ctx context.Context, prompts *PromptLibrary, input EnrichmentInput) (songstore.LyricsResult, error) {
	return songstore.LyricsResult{}, fmt.Errorf("lyrics enrichment is disabled; LRCLIB provides lyrics")
}

func (c *aiClient) testConnection(ctx context.Context) (string, error) {
	if !c.available() {
		return "", fmt.Errorf("AI is unavailable")
	}
	var parsed struct {
		OK bool `json:"ok"`
	}
	systemPrompt := "You are a connectivity test. Return valid JSON only."
	userPrompt := `{"ok":true}`
	if err := c.postJSON(ctx, systemPrompt, userPrompt, &parsed); err != nil {
		return "", err
	}
	if !parsed.OK {
		return "", fmt.Errorf("AI test returned unexpected payload")
	}
	return fmt.Sprintf("AI reachable via %s (%s)", c.model, c.baseURL), nil
}

func (c *aiClient) postJSON(ctx context.Context, systemPrompt, userPrompt string, out any) error {
	if !c.available() {
		return fmt.Errorf("AI is unavailable")
	}
	payload := map[string]any{
		"model":           c.model,
		"temperature":     0.2,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("AI request failed: %s", resp.Status)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return err
	}
	if len(parsed.Choices) == 0 {
		return fmt.Errorf("AI returned no choices")
	}
	if err := json.Unmarshal([]byte(parsed.Choices[0].Message.Content), out); err != nil {
		return err
	}
	return nil
}

type EnrichmentInput struct {
	FileName                 string
	SourceRef                string
	SourceURL                string
	SourceKind               string
	SourceTitle              string
	SourceUploader           string
	SourceChannel            string
	LibraryRoot              string
	SourceDescriptionHint    string
	DurationSeconds          *int
	DurationHint             string
	HasTranscript            bool
	HasTimestampedTranscript bool
	InputText                string
	UserContext              string
	ArtistHint               string
	AlbumHint                string
	TitleHint                string
	YearHint                 *int
	GenreHint                string
	TrackNumberHint          *int
	Notes                    string
	ResolvedTitle            string
	ResolvedArtist           string
	ResolvedAlbum            string
	ResolvedTrackNumber      string
	ResolvedYear             string
	ResolvedGenre            string
	ResolvedConfidence       string
	ResolvedNotes            string
	TargetArtistDir          string
	TargetAlbumDir           string
	TargetBaseName           string
	TargetAudioPath          string
	TargetLyricsPath         string
	TargetTimedLyricsPath    string
	TargetMetadataPath       string
}

func (e EnrichmentInput) PromptMap() map[string]string {
	sourceURL := strings.TrimSpace(e.SourceURL)
	if sourceURL == "" && looksLikeURL(e.SourceRef) {
		sourceURL = e.SourceRef
	}
	artistDir := strings.TrimSpace(e.TargetArtistDir)
	albumDir := strings.TrimSpace(e.TargetAlbumDir)
	baseName := strings.TrimSpace(e.TargetBaseName)
	audioPath := strings.TrimSpace(e.TargetAudioPath)
	lyricsPath := strings.TrimSpace(e.TargetLyricsPath)
	timedLyricsPath := strings.TrimSpace(e.TargetTimedLyricsPath)
	metadataPath := strings.TrimSpace(e.TargetMetadataPath)
	if (artistDir == "" || albumDir == "" || baseName == "" || audioPath == "" || lyricsPath == "" || timedLyricsPath == "" || metadataPath == "") && strings.TrimSpace(e.LibraryRoot) != "" {
		if derived := bestEffortPromptPaths(e.LibraryRoot, e); derived != nil {
			artistDir = firstNonEmpty(artistDir, derived.ArtistDir)
			albumDir = firstNonEmpty(albumDir, derived.AlbumDir)
			baseName = firstNonEmpty(baseName, derived.BaseName)
			audioPath = firstNonEmpty(audioPath, derived.AudioPath)
			lyricsPath = firstNonEmpty(lyricsPath, derived.LyricsPath)
			timedLyricsPath = firstNonEmpty(timedLyricsPath, derived.LRCPath)
			metadataPath = firstNonEmpty(metadataPath, derived.MetadataPath)
		}
	}
	return map[string]string{
		"FileName":                 e.FileName,
		"SourceRef":                e.SourceRef,
		"SourceURL":                sourceURL,
		"SourceKind":               e.SourceKind,
		"SourceTitle":              e.SourceTitle,
		"SourceUploader":           e.SourceUploader,
		"SourceChannel":            e.SourceChannel,
		"LibraryRoot":              e.LibraryRoot,
		"SourceDescriptionHint":    e.SourceDescriptionHint,
		"DurationSeconds":          intHint(e.DurationSeconds),
		"DurationHint":             e.DurationHint,
		"HasTranscript":            strconv.FormatBool(e.HasTranscript),
		"HasTimestampedTranscript": strconv.FormatBool(e.HasTimestampedTranscript),
		"InputText":                e.InputText,
		"UserContext":              e.UserContext,
		"ArtistHint":               e.ArtistHint,
		"AlbumHint":                e.AlbumHint,
		"TitleHint":                e.TitleHint,
		"YearHint":                 intHint(e.YearHint),
		"GenreHint":                e.GenreHint,
		"TrackNumberHint":          intHint(e.TrackNumberHint),
		"Notes":                    e.Notes,
		"ResolvedTitle":            e.ResolvedTitle,
		"ResolvedArtist":           e.ResolvedArtist,
		"ResolvedAlbum":            e.ResolvedAlbum,
		"ResolvedTrackNumber":      e.ResolvedTrackNumber,
		"ResolvedYear":             e.ResolvedYear,
		"ResolvedGenre":            e.ResolvedGenre,
		"ResolvedConfidence":       e.ResolvedConfidence,
		"ResolvedNotes":            e.ResolvedNotes,
		"TargetArtistDir":          artistDir,
		"TargetAlbumDir":           albumDir,
		"TargetBaseName":           baseName,
		"TargetAudioPath":          audioPath,
		"TargetLyricsPath":         lyricsPath,
		"TargetTimedLyricsPath":    timedLyricsPath,
		"TargetMetadataPath":       metadataPath,
	}
}

func bestEffortPromptPaths(root string, input EnrichmentInput) *songstore.SongStoragePlan {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	title := firstNonEmpty(strings.TrimSpace(input.ResolvedTitle), strings.TrimSpace(input.TitleHint), stringsTrimExt(input.FileName))
	artist := firstNonEmpty(strings.TrimSpace(input.ResolvedArtist), strings.TrimSpace(input.ArtistHint), "Unknown Artist")
	album := firstNonEmpty(strings.TrimSpace(input.ResolvedAlbum), strings.TrimSpace(input.AlbumHint), "Unknown Album")
	metadata := songstore.SongMetadata{
		Title:  title,
		Artist: artist,
		Album:  album,
	}
	if track := firstNonEmpty(input.ResolvedTrackNumber, intHint(input.TrackNumberHint)); track != "" {
		if n, err := strconv.Atoi(track); err == nil && n > 0 {
			metadata.TrackNumber = &n
		}
	}
	plan, err := songstore.BuildSongStoragePlan(root, metadata)
	if err != nil {
		return nil
	}
	return &plan
}

func looksLikeURL(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

func (e EnrichmentInput) WithResolvedMetadata(metadata songstore.MetadataResult, root string, audioName string) EnrichmentInput {
	if strings.TrimSpace(root) == "" {
		root = e.LibraryRoot
	}
	e.LibraryRoot = root
	e.ResolvedTitle = metadata.Title
	e.ResolvedArtist = metadata.Artist
	e.ResolvedAlbum = metadata.Album
	e.ResolvedConfidence = metadata.Confidence
	e.ResolvedNotes = metadata.Notes
	e.ResolvedTrackNumber = intHint(metadata.TrackNumber)
	e.ResolvedYear = intHint(metadata.Year)
	e.ResolvedGenre = metadata.Genre
	if root != "" {
		plan, err := songstore.BuildSongStoragePlan(root, songstore.SongMetadata{
			Title:       metadata.Title,
			Artist:      metadata.Artist,
			Album:       metadata.Album,
			TrackNumber: metadata.TrackNumber,
		})
		if err == nil {
			e.TargetArtistDir = plan.ArtistDir
			e.TargetAlbumDir = plan.AlbumDir
			e.TargetBaseName = plan.BaseName
			e.TargetAudioPath = plan.AudioPath
			e.TargetLyricsPath = plan.LyricsPath
			e.TargetTimedLyricsPath = plan.LRCPath
			e.TargetMetadataPath = plan.MetadataPath
		}
	} else if strings.TrimSpace(audioName) != "" {
		e.TargetBaseName = strings.TrimSuffix(filepath.Base(audioName), filepath.Ext(audioName))
		e.TargetAudioPath = audioName
	}
	return e
}

func intHint(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func fallbackMetadata(input EnrichmentInput) songstore.MetadataResult {
	artist, title := splitArtistTitle(input.TitleHint)
	if artist == "" {
		artist = input.ArtistHint
	}
	if title == "" {
		title = input.TitleHint
	}
	if artist == "" {
		artist = "Unknown Artist"
	}
	if title == "" {
		title = cleanDisplayName(stringsTrimExt(input.FileName))
	}
	album := input.AlbumHint
	if album == "" {
		album = "Unknown Album"
	}
	return songstore.MetadataResult{
		Title:              cleanDisplayName(title),
		Artist:             cleanDisplayName(artist),
		Album:              cleanDisplayName(album),
		Genre:              "",
		Confidence:         "low",
		MetadataConfidence: "low",
		Notes:              "Fallback metadata derived locally because AI enrichment was unavailable.",
	}
}

func fallbackLyrics() songstore.LyricsResult {
	return songstore.LyricsResult{
		Text:              "",
		TimedLyrics:       nil,
		TimingGranularity: "none",
		Confidence:        "low",
		Source:            "none",
		SourceLoc:         "",
		IsComplete:        false,
		Notes:             "Lyrics were unavailable.",
	}
}

func splitArtistTitle(name string) (string, string) {
	parts := strings.SplitN(name, " - ", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}
