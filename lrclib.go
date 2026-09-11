package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"melodex/internal/songstore"
)

const lrclibBaseURL = "https://lrclib.net"
const lrclibUserAgent = "Melodex/0.1 (https://melodex.local)"

var lrclibTimestampPrefix = regexp.MustCompile(`^\s*(?:\[\d{2}:\d{2}(?:\.\d{2,3})?\])+\s*`)
var lrclibSingleTimestamp = regexp.MustCompile(`\[(\d{2}):(\d{2})(?:\.(\d{2,3}))?\]`)

type lrclibRecord struct {
	ID           int     `json:"id"`
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"`
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

func fetchLyricsFromLRCLIB(ctx context.Context, baseURL string, client *http.Client, input EnrichmentInput, metadata songstore.MetadataResult) (songstore.LyricsResult, string, *songstore.MetadataResult) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = lrclibBaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	title := firstNonEmpty(strings.TrimSpace(metadata.Title), strings.TrimSpace(input.TitleHint), strings.TrimSpace(input.SourceTitle), stringsTrimExt(input.FileName))
	artist := lrclibMeaningfulTerm(metadata.Artist, input.ArtistHint, input.SourceUploader, input.SourceChannel)
	album := lrclibMeaningfulTerm(metadata.Album, input.AlbumHint)
	duration := firstDuration(metadata.DurationSeconds, input.DurationSeconds)
	log.Printf("lrclib lookup start for %s - %s", artist, title)
	logEvent("lrclib_lookup_started", "artist", artist, "title", title, "album", album, "source_url", input.SourceURL, "library_root", input.LibraryRoot)

	if title == "" || artist == "" {
		logEvent("lrclib_skipped", "artist", artist, "title", title, "reason", "missing_artist_or_title")
		return songstore.LyricsResult{
			Confidence:        "low",
			Source:            "none",
			TimingGranularity: "none",
			Notes:             "LRCLIB lookup skipped because title or artist was unavailable.",
		}, "Lyrics LRCLIB: skipped", nil
	}

	if duration != nil && album != "" {
		if record, endpoint, ok := lrclibGetCached(ctx, client, baseURL, title, artist, album, *duration); ok {
			log.Printf("lrclib cached match for %s - %s", artist, title)
			logEvent("lrclib_lookup_matched", "artist", artist, "title", title, "album", album, "endpoint", endpoint)
			return lyricsResultFromLRCLIBRecord(record, endpoint), "Lyrics LRCLIB: cached match", lrclibMetadataResult(record, metadata)
		}
		if record, endpoint, ok := lrclibGet(ctx, client, baseURL, title, artist, album, *duration); ok {
			log.Printf("lrclib direct match for %s - %s", artist, title)
			logEvent("lrclib_lookup_matched", "artist", artist, "title", title, "album", album, "endpoint", endpoint)
			return lyricsResultFromLRCLIBRecord(record, endpoint), "Lyrics LRCLIB: matched", lrclibMetadataResult(record, metadata)
		}
	}

	if record, endpoint, ok := lrclibSearch(ctx, client, baseURL, title, artist, album, duration); ok {
		log.Printf("lrclib search match for %s - %s", artist, title)
		logEvent("lrclib_lookup_matched", "artist", artist, "title", title, "album", album, "endpoint", endpoint)
		return lyricsResultFromLRCLIBRecord(record, endpoint), "Lyrics LRCLIB: matched", lrclibMetadataResult(record, metadata)
	}
	log.Printf("lrclib no match for %s - %s", artist, title)
	logEvent("lrclib_lookup_missed", "artist", artist, "title", title, "album", album)

	return songstore.LyricsResult{
		Confidence:        "low",
		Source:            "none",
		TimingGranularity: "none",
		Notes:             "LRCLIB did not return a match for this track.",
	}, "Lyrics LRCLIB: not found", nil
}

func lrclibGetCached(ctx context.Context, client *http.Client, baseURL, title, artist, album string, duration int) (lrclibRecord, string, bool) {
	return lrclibGetLike(ctx, client, baseURL, "get-cached", title, artist, album, duration)
}

func lrclibGet(ctx context.Context, client *http.Client, baseURL, title, artist, album string, duration int) (lrclibRecord, string, bool) {
	return lrclibGetLike(ctx, client, baseURL, "get", title, artist, album, duration)
}

func lrclibGetLike(ctx context.Context, client *http.Client, baseURL, endpoint, title, artist, album string, duration int) (lrclibRecord, string, bool) {
	requestURL := fmt.Sprintf("%s/api/%s?track_name=%s&artist_name=%s&album_name=%s&duration=%d", baseURL, endpoint, url.QueryEscape(title), url.QueryEscape(artist), url.QueryEscape(album), duration)
	record, ok, err := lrclibGetRecord(ctx, client, requestURL)
	if err != nil {
		log.Printf("LRCLIB %s lookup failed for %s - %s: %v", endpoint, artist, title, err)
		logEvent("lrclib_lookup_failed", "endpoint", endpoint, "artist", artist, "title", title, "request_url", requestURL, "error", err.Error())
		return lrclibRecord{}, requestURL, false
	}
	return record, requestURL, ok
}

func lrclibSearch(ctx context.Context, client *http.Client, baseURL, title, artist, album string, duration *int) (lrclibRecord, string, bool) {
	queries := lrclibSearchQueries(title, artist, album)
	var candidates []lrclibRecord
	for _, query := range queries {
		requestURL := fmt.Sprintf("%s/api/search?%s", baseURL, query.Encode())
		records, err := lrclibGetSearchRecords(ctx, client, requestURL)
		if err != nil {
			log.Printf("LRCLIB search failed for %s - %s: %v", artist, title, err)
			logEvent("lrclib_search_failed", "artist", artist, "title", title, "request_url", requestURL, "error", err.Error())
			continue
		}
		candidates = append(candidates, records...)
	}
	if len(candidates) == 0 {
		return lrclibRecord{}, "", false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return lrclibRecordScore(candidates[i], title, artist, album, duration) > lrclibRecordScore(candidates[j], title, artist, album, duration)
	})
	record := candidates[0]
	return record, fmt.Sprintf("%s/api/get/%d", baseURL, record.ID), true
}

func lrclibSearchQueries(title, artist, album string) []url.Values {
	queries := make([]url.Values, 0, 3)
	if title != "" && artist != "" && album != "" {
		q := url.Values{}
		q.Set("track_name", title)
		q.Set("artist_name", artist)
		q.Set("album_name", album)
		queries = append(queries, q)
	}
	if title != "" && artist != "" {
		q := url.Values{}
		q.Set("track_name", title)
		q.Set("artist_name", artist)
		queries = append(queries, q)
	}
	if title != "" {
		q := url.Values{}
		q.Set("q", strings.TrimSpace(strings.Join(nonEmptyLRCLIBTerms(title, artist, album), " ")))
		queries = append(queries, q)
	}
	return queries
}

func lrclibGetRecord(ctx context.Context, client *http.Client, requestURL string) (lrclibRecord, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return lrclibRecord{}, false, err
	}
	req.Header.Set("User-Agent", lrclibUserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return lrclibRecord{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return lrclibRecord{}, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return lrclibRecord{}, false, fmt.Errorf("LRCLIB returned %s", resp.Status)
	}
	var record lrclibRecord
	if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
		return lrclibRecord{}, false, err
	}
	if record.ID == 0 && strings.TrimSpace(record.TrackName) == "" {
		return lrclibRecord{}, false, nil
	}
	return record, true, nil
}

func lrclibGetSearchRecords(ctx context.Context, client *http.Client, requestURL string) ([]lrclibRecord, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", lrclibUserAgent)
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
		return nil, fmt.Errorf("LRCLIB returned %s", resp.Status)
	}
	var records []lrclibRecord
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
		return nil, err
	}
	return records, nil
}

func lrclibRecordScore(record lrclibRecord, title, artist, album string, duration *int) int {
	score := 0
	if strings.EqualFold(strings.TrimSpace(record.TrackName), strings.TrimSpace(title)) {
		score += 6
	}
	if strings.EqualFold(strings.TrimSpace(record.ArtistName), strings.TrimSpace(artist)) {
		score += 6
	}
	if album != "" && strings.EqualFold(strings.TrimSpace(record.AlbumName), strings.TrimSpace(album)) {
		score += 4
	}
	if duration != nil && record.Duration > 0 {
		delta := record.Duration - float64(*duration)
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
	if record.Instrumental {
		score--
	}
	return score
}

func lrclibMeaningfulTerm(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		switch lower {
		case "unknown", "unknown artist", "unknown album", "n/a", "na", "none":
			continue
		}
		return trimmed
	}
	return ""
}

func lrclibShouldReplaceAlbum(value string) bool {
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

func nonEmptyLRCLIBTerms(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func lrclibMetadataResult(record lrclibRecord, fallback songstore.MetadataResult) *songstore.MetadataResult {
	merged := fallback
	if title := strings.TrimSpace(record.TrackName); title != "" {
		merged.Title = title
	}
	if artist := strings.TrimSpace(record.ArtistName); artist != "" {
		merged.Artist = artist
	}
	if album := strings.TrimSpace(record.AlbumName); album != "" && lrclibShouldReplaceAlbum(merged.Album) {
		merged.Album = album
	}
	if record.Duration > 0 {
		duration := int(record.Duration)
		merged.DurationSeconds = &duration
	}
	if strings.TrimSpace(merged.Confidence) == "" {
		merged.Confidence = "high"
	}
	if strings.TrimSpace(merged.Notes) == "" {
		merged.Notes = "LRCLIB matched canonical track metadata."
	} else if !strings.Contains(strings.ToLower(merged.Notes), "lrclib matched canonical track metadata") {
		merged.Notes = strings.TrimSpace(merged.Notes + " LRCLIB matched canonical track metadata.")
	}
	return &merged
}

func lyricsResultFromLRCLIBRecord(record lrclibRecord, sourceLoc string) songstore.LyricsResult {
	plainLyrics := strings.TrimSpace(record.PlainLyrics)
	timedLyrics := parseLRCLIBSyncedLyrics(record.SyncedLyrics)
	if plainLyrics == "" && len(timedLyrics) > 0 {
		plainLyrics = plainLyricsFromSyncedLyrics(record.SyncedLyrics)
	}
	confidence := "medium"
	if len(timedLyrics) > 0 || plainLyrics != "" {
		confidence = "high"
	}
	if record.Instrumental {
		confidence = "low"
	}
	return songstore.LyricsResult{
		Text:              plainLyrics,
		TimedLyrics:       timedLyrics,
		TimingGranularity: lyricGranularityForLines(timedLyrics),
		Confidence:        confidence,
		Source:            "lrclib",
		SourceLoc:         sourceLoc,
		IsComplete:        !record.Instrumental && (plainLyrics != "" || len(timedLyrics) > 0),
		Notes:             lrclibNotes(record, plainLyrics, timedLyrics),
	}
}

func parseLRCLIBSyncedLyrics(text string) []songstore.TimedLyricLine {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]songstore.TimedLyricLine, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		stamps := lrclibTimestampPrefix.FindString(line)
		if stamps == "" {
			continue
		}
		trimmed := strings.TrimSpace(lrclibTimestampPrefix.ReplaceAllString(line, ""))
		matches := lrclibSingleTimestamp.FindAllStringSubmatch(stamps, -1)
		for _, match := range matches {
			seconds := parseLRCLIBTimestamp(match)
			out = append(out, songstore.TimedLyricLine{StartSeconds: seconds, Text: trimmed})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].StartSeconds < out[j].StartSeconds
	})
	return out
}

func parseLRCLIBTimestamp(match []string) float64 {
	if len(match) < 3 {
		return 0
	}
	minutes, _ := strconv.Atoi(match[1])
	seconds, _ := strconv.Atoi(match[2])
	fraction := 0.0
	if len(match) > 3 && match[3] != "" {
		if v, err := strconv.Atoi(match[3]); err == nil {
			switch len(match[3]) {
			case 1:
				fraction = float64(v) / 10
			case 2:
				fraction = float64(v) / 100
			default:
				fraction = float64(v) / 1000
			}
		}
	}
	return float64(minutes*60+seconds) + fraction
}

func lyricGranularityForLines(lines []songstore.TimedLyricLine) string {
	if len(lines) == 0 {
		return "none"
	}
	return "line"
}

func plainLyricsFromSyncedLyrics(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(lrclibTimestampPrefix.ReplaceAllString(line, ""))
		out = append(out, trimmed)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func lrclibNotes(record lrclibRecord, plainLyrics string, timedLyrics []songstore.TimedLyricLine) string {
	switch {
	case record.Instrumental:
		return "LRCLIB marked this track as instrumental."
	case len(timedLyrics) > 0:
		return "Lyrics fetched from LRCLIB with synced timing."
	case plainLyrics != "":
		return "Lyrics fetched from LRCLIB without timing."
	default:
		return "LRCLIB returned no lyrics text."
	}
}
