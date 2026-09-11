package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"melodex/internal/songstore"
)

func TestFetchMetadataFromMusicBrainzUsesRecordingSearch(t *testing.T) {
	t.Parallel()

	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.String())
		if r.URL.Path != "/ws/2/recording/" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query().Get("query")
		if !strings.Contains(query, `recording:"The Red"`) {
			t.Fatalf("missing recording query: %q", query)
		}
		if !strings.Contains(query, `artistname:"Chevelle"`) {
			t.Fatalf("missing artist query: %q", query)
		}
		fmt.Fprint(w, `{
			"created": "2026-05-25T00:00:00Z",
			"count": 1,
			"offset": 0,
			"recordings": [
				{
					"id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
					"score": "100",
					"title": "The Red",
					"length": 243000,
					"artist-credit": [
						{
							"artist": {
								"id": "11111111-2222-3333-4444-555555555555",
								"name": "Chevelle",
								"sort-name": "Chevelle"
							}
						}
					],
					"first-release-date": "2002-09-24",
					"releases": [
						{
							"id": "99999999-8888-7777-6666-555555555555",
							"title": "Wonder What's Next",
							"status": "Official",
							"date": "2002-09-24",
							"release-group": {
								"id": "44444444-3333-2222-1111-000000000000",
								"primary-type": "Album"
							}
						}
					]
				}
			]
		}`)
	}))
	defer server.Close()

	duration := 243
	input := EnrichmentInput{
		TitleHint:       "The Red",
		ArtistHint:      "Chevelle",
		AlbumHint:       "Unknown Album",
		DurationSeconds: &duration,
		FileName:        "The Red.mp3",
	}
	fallback := songstore.MetadataResult{
		Title:           "The Red",
		Artist:          "Chevelle",
		Album:           "Unknown Album",
		DurationSeconds: &duration,
		Confidence:      "low",
	}

	metadata, detail, ok := fetchMetadataFromMusicBrainzAt(context.Background(), server.URL, input, fallback)
	if !ok {
		t.Fatalf("expected a match")
	}
	if detail != "matched" {
		t.Fatalf("unexpected detail: %q", detail)
	}
	if got, want := metadata.Title, "The Red"; got != want {
		t.Fatalf("unexpected title: got %q want %q", got, want)
	}
	if got, want := metadata.Artist, "Chevelle"; got != want {
		t.Fatalf("unexpected artist: got %q want %q", got, want)
	}
	if got, want := metadata.Album, "Wonder What's Next"; got != want {
		t.Fatalf("unexpected album: got %q want %q", got, want)
	}
	if metadata.Year == nil || *metadata.Year != 2002 {
		t.Fatalf("unexpected year: %#v", metadata.Year)
	}
	if metadata.DurationSeconds == nil || *metadata.DurationSeconds != 243 {
		t.Fatalf("unexpected duration: %#v", metadata.DurationSeconds)
	}
	if !strings.Contains(metadata.Notes, "MusicBrainz matched canonical track metadata.") {
		t.Fatalf("expected MusicBrainz note, got %q", metadata.Notes)
	}
	if len(requested) == 0 {
		t.Fatalf("expected a request")
	}
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	if parsed.Host == "" {
		t.Fatalf("expected server host")
	}
}

func TestMusicBrainzBestReleasePrefersAlbum(t *testing.T) {
	t.Parallel()

	album := musicBrainzRelease{
		Title:  "Wonder What's Next",
		Status: "Official",
		Date:   "2002-09-24",
		ReleaseGroup: musicBrainzReleaseGroup{
			PrimaryType: "Album",
		},
	}
	single := musicBrainzRelease{
		Title:  "The Red",
		Status: "Official",
		Date:   "2002-06-11",
		ReleaseGroup: musicBrainzReleaseGroup{
			PrimaryType: "Single",
		},
	}

	best := musicBrainzBestRelease([]musicBrainzRelease{single, album})
	if best == nil {
		t.Fatalf("expected best release")
	}
	if got, want := best.Title, "Wonder What's Next"; got != want {
		t.Fatalf("unexpected best release: got %q want %q", got, want)
	}
}
