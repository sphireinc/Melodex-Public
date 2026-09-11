package main

import (
	"strings"
	"testing"

	"melodex/internal/songstore"
)

func TestJobStageStatusesAndCompletionDetailIncludeVideoState(t *testing.T) {
	job := Job{
		Status:           "running",
		Detail:           "Download: downloaded; Metadata: metadata discovered; MusicBrainz: matched; Lyrics: matched; Album Art: downloaded; Video: downloaded; Finalize: completed",
		DownloadProgress: 100,
		MetadataProgress: 100,
		LyricsProgress:   100,
		ResultTitle:      "Song",
		ResultArtist:     "Artist",
		ResultAlbum:      "Album",
	}
	stages := jobStageStatusesFor(job)
	if stages.Download != "Downloaded" {
		t.Fatalf("expected download stage to normalize, got %q", stages.Download)
	}
	if stages.Metadata != "Metadata discovered" {
		t.Fatalf("expected metadata stage to normalize, got %q", stages.Metadata)
	}
	if stages.MusicBrainz != "Matched" {
		t.Fatalf("expected MusicBrainz stage to normalize, got %q", stages.MusicBrainz)
	}
	if stages.Video != "Downloaded" {
		t.Fatalf("expected video stage to normalize, got %q", stages.Video)
	}
	if stages.Finalize != "Completed" {
		t.Fatalf("expected finalize stage to normalize, got %q", stages.Finalize)
	}

	detail := buildCompletedJobDetail(
		"url",
		aiOutcome{Ran: true, Success: true, Status: "success", Message: "Metadata discovered"},
		"matched",
		songstore.LyricsResult{Source: "lrclib"},
		"matched",
		songstore.SongMetadata{
			Title:  "Song",
			Artist: "Artist",
			Album:  "Album",
			Video:  songstore.SongVideo{Path: "/tmp/song.mp4"},
		},
	)
	if !strings.Contains(detail, "Video: Downloaded") {
		t.Fatalf("expected completion detail to include downloaded video state, got %q", detail)
	}
	if !strings.Contains(detail, "Url") {
		t.Fatalf("expected completion detail to preserve source label, got %q", detail)
	}
}
