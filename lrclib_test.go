package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"melodex/internal/songstore"
)

func TestParseLRCLIBSyncedLyrics(t *testing.T) {
	t.Parallel()

	lines := parseLRCLIBSyncedLyrics("[00:01.00]Hello\n[00:02.50]World\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if got, want := lines[0].StartSeconds, 1.0; got != want {
		t.Fatalf("unexpected first timestamp: got %v want %v", got, want)
	}
	if got, want := lines[0].Text, "Hello"; got != want {
		t.Fatalf("unexpected first text: got %q want %q", got, want)
	}
	if got, want := lines[1].StartSeconds, 2.5; got != want {
		t.Fatalf("unexpected second timestamp: got %v want %v", got, want)
	}
	if got, want := lines[1].Text, "World"; got != want {
		t.Fatalf("unexpected second text: got %q want %q", got, want)
	}
}

func TestFetchLyricsFromLRCLIBUsesCachedEndpoint(t *testing.T) {
	t.Parallel()

	var requested []string
	server := newLRCLIBTCP4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.String())
		if r.URL.Path != "/api/get-cached" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("track_name"); got != "Comfortable Liar" {
			t.Fatalf("unexpected track_name: %q", got)
		}
		if got := r.URL.Query().Get("artist_name"); got != "Chevelle" {
			t.Fatalf("unexpected artist_name: %q", got)
		}
		if got := r.URL.Query().Get("album_name"); got != "Wonder What's Next" {
			t.Fatalf("unexpected album_name: %q", got)
		}
		if got := r.URL.Query().Get("duration"); got != "225" {
			t.Fatalf("unexpected duration: %q", got)
		}
		fmt.Fprint(w, `{
			"id": 42,
			"trackName": "Comfortable Liar",
			"artistName": "Chevelle",
			"albumName": "Wonder What's Next",
			"duration": 225,
			"plainLyrics": "",
			"syncedLyrics": "[00:01.00]Hello\n[00:02.50]World",
			"instrumental": false
		}`)
	}))
	defer server.Close()

	duration := 225
	input := EnrichmentInput{
		TitleHint:       "Comfortable Liar",
		ArtistHint:      "Chevelle",
		AlbumHint:       "Wonder What's Next",
		DurationSeconds: &duration,
	}
	metadata := songstore.MetadataResult{
		Title:           "Comfortable Liar",
		Artist:          "Chevelle",
		Album:           "Wonder What's Next",
		DurationSeconds: &duration,
	}

	result, detail, merged := fetchLyricsFromLRCLIB(context.Background(), server.URL, server.Client(), input, metadata)
	if detail != "Lyrics LRCLIB: cached match" {
		t.Fatalf("unexpected detail: %q", detail)
	}
	if result.Source != "lrclib" {
		t.Fatalf("unexpected source: %q", result.Source)
	}
	parsed, err := url.Parse(result.SourceLoc)
	if err != nil {
		t.Fatalf("parse source loc: %v", err)
	}
	if parsed.Path != "/api/get-cached" {
		t.Fatalf("unexpected source loc path: %q", parsed.Path)
	}
	if got := parsed.Query().Get("track_name"); got != "Comfortable Liar" {
		t.Fatalf("unexpected source loc track_name: %q", got)
	}
	if got := parsed.Query().Get("artist_name"); got != "Chevelle" {
		t.Fatalf("unexpected source loc artist_name: %q", got)
	}
	if got := parsed.Query().Get("album_name"); got != "Wonder What's Next" {
		t.Fatalf("unexpected source loc album_name: %q", got)
	}
	if got := parsed.Query().Get("duration"); got != "225" {
		t.Fatalf("unexpected source loc duration: %q", got)
	}
	if result.Confidence != "high" {
		t.Fatalf("unexpected confidence: %q", result.Confidence)
	}
	if !result.IsComplete {
		t.Fatalf("expected complete lyrics")
	}
	if len(result.TimedLyrics) != 2 {
		t.Fatalf("expected 2 timed lines, got %d", len(result.TimedLyrics))
	}
	if text := strings.TrimSpace(result.Text); text != "Hello\nWorld" {
		t.Fatalf("unexpected lyrics text: %q", text)
	}
	if len(requested) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requested))
	}
	if merged == nil {
		t.Fatalf("expected merged metadata")
	}
	if got, want := merged.Title, "Comfortable Liar"; got != want {
		t.Fatalf("unexpected merged title: got %q want %q", got, want)
	}
	if got, want := merged.Album, "Wonder What's Next"; got != want {
		t.Fatalf("unexpected merged album: got %q want %q", got, want)
	}
}

func TestFetchLyricsFromLRCLIBSkipsWithoutMetadata(t *testing.T) {
	t.Parallel()

	result, detail, merged := fetchLyricsFromLRCLIB(context.Background(), "https://lrclib.net", nil, EnrichmentInput{}, songstore.MetadataResult{})
	if detail != "Lyrics LRCLIB: skipped" {
		t.Fatalf("unexpected detail: %q", detail)
	}
	if result.Source != "none" {
		t.Fatalf("unexpected source: %q", result.Source)
	}
	if result.Notes == "" {
		t.Fatalf("expected skip note")
	}
	if merged != nil {
		t.Fatalf("expected no merged metadata")
	}
}

func TestFetchLyricsFromLRCLIBIgnoresPlaceholderAlbum(t *testing.T) {
	t.Parallel()

	var requested []string
	server := newLRCLIBTCP4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.String())
		if r.URL.Path != "/api/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("album_name"); got != "" {
			t.Fatalf("unexpected album_name: %q", got)
		}
		if got := r.URL.Query().Get("track_name"); got != "The Red" && got != "" {
			t.Fatalf("unexpected track_name: %q", got)
		}
		if got := r.URL.Query().Get("artist_name"); got != "Chevelle" && got != "" {
			t.Fatalf("unexpected artist_name: %q", got)
		}
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()

	duration := 243
	input := EnrichmentInput{
		TitleHint:       "The Red",
		ArtistHint:      "Chevelle",
		AlbumHint:       "Unknown Album",
		DurationSeconds: &duration,
	}
	metadata := songstore.MetadataResult{
		Title:           "The Red",
		Artist:          "Chevelle",
		Album:           "Unknown Album",
		DurationSeconds: &duration,
	}

	result, detail, merged := fetchLyricsFromLRCLIB(context.Background(), server.URL, server.Client(), input, metadata)
	if detail != "Lyrics LRCLIB: not found" {
		t.Fatalf("unexpected detail: %q", detail)
	}
	if result.Source != "none" {
		t.Fatalf("unexpected source: %q", result.Source)
	}
	if len(requested) == 0 {
		t.Fatalf("expected search requests")
	}
	if merged != nil {
		t.Fatalf("expected no merged metadata on miss")
	}
}

func newLRCLIBTCP4TestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for LRCLIB test server: %v", err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	return server
}

func TestLRCLIBDoesNotDowngradeKnownAlbum(t *testing.T) {
	t.Parallel()

	record := lrclibRecord{
		TrackName:  "The Red",
		ArtistName: "Chevelle",
		AlbumName:  "The Red",
		Duration:   243,
	}
	fallback := songstore.MetadataResult{
		Title:           "The Red",
		Artist:          "Chevelle",
		Album:           "Wonder What's Next",
		Confidence:      "medium",
		DurationSeconds: func() *int { v := 243; return &v }(),
	}

	merged := lrclibMetadataResult(record, fallback)
	if merged == nil {
		t.Fatalf("expected merged metadata")
	}
	if got, want := merged.Album, "Wonder What's Next"; got != want {
		t.Fatalf("unexpected album: got %q want %q", got, want)
	}
}
