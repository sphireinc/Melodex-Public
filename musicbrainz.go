package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"melodex/internal/songstore"
)

const musicBrainzBaseURL = "https://musicbrainz.org"

type musicBrainzRecordingSearchResponse struct {
	Recordings []musicBrainzRecording `json:"recordings"`
}

type musicBrainzRecording struct {
	ID               string                    `json:"id"`
	Score            musicBrainzSearchScore    `json:"score"`
	Title            string                    `json:"title"`
	Length           *int                      `json:"length"`
	ArtistCredit     []musicBrainzArtistCredit `json:"artist-credit"`
	FirstReleaseDate string                    `json:"first-release-date"`
	Releases         []musicBrainzRelease      `json:"releases"`
}

type musicBrainzSearchScore int

func (s *musicBrainzSearchScore) UnmarshalJSON(data []byte) error {
	var asInt int
	if err := json.Unmarshal(data, &asInt); err == nil {
		*s = musicBrainzSearchScore(asInt)
		return nil
	}
	var asString string
	if err := json.Unmarshal(data, &asString); err == nil {
		if asString == "" {
			*s = 0
			return nil
		}
		parsed, err := strconv.Atoi(asString)
		if err != nil {
			return err
		}
		*s = musicBrainzSearchScore(parsed)
		return nil
	}
	return fmt.Errorf("invalid MusicBrainz score")
}

type musicBrainzArtistCredit struct {
	JoinPhrase string `json:"joinphrase"`
	Artist     struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"artist"`
}

type musicBrainzRelease struct {
	ID           string                  `json:"id"`
	Title        string                  `json:"title"`
	Status       string                  `json:"status"`
	Date         string                  `json:"date"`
	ReleaseGroup musicBrainzReleaseGroup `json:"release-group"`
}

type musicBrainzReleaseGroup struct {
	ID             string   `json:"id"`
	PrimaryType    string   `json:"primary-type"`
	SecondaryTypes []string `json:"secondary-types"`
}

func fetchMetadataFromMusicBrainz(ctx context.Context, input EnrichmentInput, metadata songstore.MetadataResult) (songstore.MetadataResult, string, bool) {
	return fetchMetadataFromMusicBrainzAt(ctx, musicBrainzBaseURL, input, metadata)
}

func fetchMetadataFromMusicBrainzAt(ctx context.Context, baseURL string, input EnrichmentInput, metadata songstore.MetadataResult) (songstore.MetadataResult, string, bool) {
	client := &http.Client{Timeout: 20 * time.Second}
	title := musicBrainzMeaningfulTerm(metadata.Title, input.TitleHint, input.SourceTitle, stringsTrimExt(input.FileName))
	artist := musicBrainzMeaningfulTerm(metadata.Artist, input.ArtistHint, input.SourceUploader, input.SourceChannel)
	album := musicBrainzMeaningfulTerm(metadata.Album, input.AlbumHint)
	duration := firstDuration(metadata.DurationSeconds, input.DurationSeconds)
	log.Printf("musicbrainz lookup start for %s - %s", artist, title)
	logEvent("musicbrainz_lookup_started", "artist", artist, "title", title, "album", album, "source_url", input.SourceURL, "library_root", input.LibraryRoot)

	if title == "" || artist == "" {
		logEvent("musicbrainz_skipped", "artist", artist, "title", title, "reason", "missing_artist_or_title")
		return metadata, "skipped", false
	}

	record, requestURL, ok := musicBrainzLookupRecording(ctx, client, baseURL, title, artist, album, duration)
	if !ok {
		if requestURL != "" {
			log.Printf("MusicBrainz lookup did not find a match for %s - %s (%s)", artist, title, requestURL)
			logEvent("musicbrainz_lookup_missed", "artist", artist, "title", title, "request_url", requestURL)
		}
		return metadata, "not found", false
	}

	merged := musicBrainzMetadataResult(record, metadata)
	log.Printf("musicbrainz matched release for %s - %s", artist, title)
	year := ""
	if merged.Year != nil {
		year = strconv.Itoa(*merged.Year)
	}
	logEvent("musicbrainz_lookup_matched", "artist", artist, "title", title, "album", merged.Album, "year", year)
	return merged, "matched", true
}

func musicBrainzLookupRecording(ctx context.Context, client *http.Client, baseURL, title, artist, album string, duration *int) (musicBrainzRecording, string, bool) {
	queries := musicBrainzRecordingQueries(title, artist, album, duration)
	var candidates []musicBrainzRecording
	var lastRequestURL string
	for _, query := range queries {
		requestURL := fmt.Sprintf("%s/ws/2/recording/?fmt=json&limit=10&query=%s", strings.TrimRight(strings.TrimSpace(baseURL), "/"), url.QueryEscape(query))
		lastRequestURL = requestURL
		records, err := musicBrainzGetSearchRecordings(ctx, client, requestURL)
		if err != nil {
			log.Printf("MusicBrainz search failed for %s - %s: %v", artist, title, err)
			logEvent("musicbrainz_search_failed", "artist", artist, "title", title, "request_url", requestURL, "error", err.Error())
			continue
		}
		candidates = append(candidates, records...)
	}
	if len(candidates) == 0 {
		return musicBrainzRecording{}, lastRequestURL, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return musicBrainzRecordingScore(candidates[i], title, artist, album, duration) > musicBrainzRecordingScore(candidates[j], title, artist, album, duration)
	})
	return candidates[0], lastRequestURL, true
}

func musicBrainzRecordingQueries(title, artist, album string, duration *int) []string {
	title = strings.TrimSpace(title)
	artist = musicBrainzMeaningfulTerm(artist)
	album = musicBrainzMeaningfulTerm(album)

	queries := make([]string, 0, 4)
	appendQuery := func(includeAlbum, includeDuration bool) {
		terms := make([]string, 0, 4)
		if title != "" {
			terms = append(terms, fmt.Sprintf(`recording:"%s"`, escapeMusicBrainzQuery(title)))
		}
		if artist != "" {
			terms = append(terms, fmt.Sprintf(`artistname:"%s"`, escapeMusicBrainzQuery(artist)))
		}
		if includeAlbum && album != "" {
			terms = append(terms, fmt.Sprintf(`release:"%s"`, escapeMusicBrainzQuery(album)))
		}
		if includeDuration && duration != nil && *duration > 0 {
			terms = append(terms, fmt.Sprintf(`dur:%d`, (*duration)*1000))
		}
		if len(terms) > 0 {
			queries = append(queries, strings.Join(terms, " AND "))
		}
	}

	appendQuery(true, true)
	appendQuery(true, false)
	appendQuery(false, true)
	appendQuery(false, false)

	return uniqueStrings(queries)
}

func musicBrainzGetSearchRecordings(ctx context.Context, client *http.Client, requestURL string) ([]musicBrainzRecording, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", musicBrainzUserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MusicBrainz returned %s", resp.Status)
	}
	var payload musicBrainzRecordingSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Recordings, nil
}

func musicBrainzRecordingScore(record musicBrainzRecording, title, artist, album string, duration *int) int {
	score := 0
	if strings.EqualFold(strings.TrimSpace(record.Title), strings.TrimSpace(title)) {
		score += 8
	}
	if musicBrainzArtistCreditString(record.ArtistCredit) != "" && strings.EqualFold(strings.TrimSpace(musicBrainzArtistCreditString(record.ArtistCredit)), strings.TrimSpace(artist)) {
		score += 8
	}
	if release := musicBrainzBestRelease(record.Releases); release != nil {
		switch strings.ToLower(strings.TrimSpace(release.ReleaseGroup.PrimaryType)) {
		case "album":
			score += 8
		case "ep":
			score += 5
		case "single":
			score += 3
		case "soundtrack", "broadcast":
			score += 2
		default:
			score++
		}
		if strings.EqualFold(strings.TrimSpace(release.Title), strings.TrimSpace(album)) {
			score += 4
		}
		if strings.EqualFold(strings.TrimSpace(release.Status), "official") {
			score += 2
		}
		if releaseHasCompilation(release.ReleaseGroup.SecondaryTypes) {
			score--
		}
	}
	if duration != nil && record.Length != nil && *duration > 0 {
		recordSeconds := float64(*record.Length) / 1000.0
		delta := recordSeconds - float64(*duration)
		if delta < 0 {
			delta = -delta
		}
		switch {
		case delta == 0:
			score += 6
		case delta <= 2:
			score += 4
		case delta <= 5:
			score += 2
		}
	}
	if strings.TrimSpace(record.FirstReleaseDate) != "" {
		score++
	}
	return score
}

func musicBrainzMetadataResult(record musicBrainzRecording, fallback songstore.MetadataResult) songstore.MetadataResult {
	merged := fallback
	if title := cleanDisplayName(strings.TrimSpace(record.Title)); title != "" {
		merged.Title = title
	}
	if artist := cleanDisplayName(strings.TrimSpace(musicBrainzArtistCreditString(record.ArtistCredit))); artist != "" {
		merged.Artist = artist
	}
	if release := musicBrainzBestRelease(record.Releases); release != nil {
		album := cleanDisplayName(strings.TrimSpace(release.Title))
		if musicBrainzShouldReplaceAlbum(merged.Album) && album != "" {
			merged.Album = album
		}
		if releaseType := strings.ToLower(strings.TrimSpace(release.ReleaseGroup.PrimaryType)); releaseType != "" && merged.ReleaseType == "" {
			merged.ReleaseType = releaseType
		}
		if year := musicBrainzYearFromDate(release.Date); year != nil && merged.Year == nil {
			merged.Year = year
		}
		if merged.DurationSeconds == nil && record.Length != nil && *record.Length > 0 {
			duration := int((*record.Length + 500) / 1000)
			merged.DurationSeconds = &duration
		}
		if strings.TrimSpace(merged.MetadataConfidence) == "" || strings.EqualFold(strings.TrimSpace(merged.MetadataConfidence), "low") {
			merged.MetadataConfidence = musicBrainzConfidence(release)
		}
		if strings.TrimSpace(merged.Confidence) == "" || strings.EqualFold(strings.TrimSpace(merged.Confidence), "low") {
			merged.Confidence = merged.MetadataConfidence
		}
		notes := []string{"MusicBrainz matched canonical track metadata."}
		if album != "" {
			notes = append(notes, "Preferred release: "+album)
		}
		if merged.Notes != "" {
			notes = append([]string{strings.TrimSpace(merged.Notes)}, notes...)
		}
		merged.Notes = strings.TrimSpace(strings.Join(notes, " "))
	} else {
		merged.Notes = appendMetadataNote(merged.Notes, "MusicBrainz matched canonical track metadata.")
		if strings.TrimSpace(merged.MetadataConfidence) == "" || strings.EqualFold(strings.TrimSpace(merged.MetadataConfidence), "low") {
			merged.MetadataConfidence = "medium"
		}
		if strings.TrimSpace(merged.Confidence) == "" || strings.EqualFold(strings.TrimSpace(merged.Confidence), "low") {
			merged.Confidence = merged.MetadataConfidence
		}
	}
	return merged
}

func musicBrainzArtistCreditString(credits []musicBrainzArtistCredit) string {
	if len(credits) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, credit := range credits {
		name := strings.TrimSpace(credit.Artist.Name)
		if name == "" {
			continue
		}
		builder.WriteString(name)
		builder.WriteString(credit.JoinPhrase)
	}
	return strings.TrimSpace(builder.String())
}

func musicBrainzBestRelease(releases []musicBrainzRelease) *musicBrainzRelease {
	if len(releases) == 0 {
		return nil
	}
	candidates := append([]musicBrainzRelease(nil), releases...)
	sort.SliceStable(candidates, func(i, j int) bool {
		return musicBrainzReleaseScore(candidates[i]) > musicBrainzReleaseScore(candidates[j])
	})
	return &candidates[0]
}

func musicBrainzConfidence(release *musicBrainzRelease) string {
	if release == nil {
		return "medium"
	}
	primary := strings.ToLower(strings.TrimSpace(release.ReleaseGroup.PrimaryType))
	status := strings.ToLower(strings.TrimSpace(release.Status))
	switch {
	case primary == "album" && status == "official":
		return "high"
	case primary == "album", primary == "ep", primary == "single":
		return "medium"
	default:
		return "medium"
	}
}

func musicBrainzReleaseScore(release musicBrainzRelease) int {
	score := 0
	switch strings.ToLower(strings.TrimSpace(release.ReleaseGroup.PrimaryType)) {
	case "album":
		score += 40
	case "ep":
		score += 20
	case "single":
		score += 8
	case "soundtrack", "broadcast":
		score += 4
	default:
		score += 2
	}
	if strings.EqualFold(strings.TrimSpace(release.Status), "official") {
		score += 6
	}
	if releaseHasCompilation(release.ReleaseGroup.SecondaryTypes) {
		score -= 4
	}
	if strings.TrimSpace(release.Title) != "" {
		score += 2
	}
	if strings.TrimSpace(release.Date) != "" {
		score += 2
	}
	return score
}

func releaseHasCompilation(values []string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), "Compilation") {
			return true
		}
	}
	return false
}

func musicBrainzShouldReplaceAlbum(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	switch strings.ToLower(value) {
	case "unknown", "unknown album", "n/a", "na", "none":
		return true
	default:
		return false
	}
}

func musicBrainzYearFromDate(value string) *int {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.SplitN(value, "-", 2)
	if len(parts) == 0 {
		return nil
	}
	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil
	}
	return &year
}

func musicBrainzMeaningfulTerm(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		switch strings.ToLower(trimmed) {
		case "unknown", "unknown artist", "unknown album", "n/a", "na", "none":
			continue
		}
		return trimmed
	}
	return ""
}

func appendMetadataNote(existing, note string) string {
	existing = strings.TrimSpace(existing)
	note = strings.TrimSpace(note)
	switch {
	case existing == "":
		return note
	case note == "":
		return existing
	case strings.Contains(strings.ToLower(existing), strings.ToLower(note)):
		return existing
	default:
		return strings.TrimSpace(existing + " " + note)
	}
}
