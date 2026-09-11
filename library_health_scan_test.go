package main

import (
	"os"
	"reflect"
	"testing"
)

func TestScanLibraryHealthIsDeterministicAndReadOnly(t *testing.T) {
	tracks := []TrackRecord{
		{ID: "b", Artist: "Artist", Title: "Song", AudioPath: "/missing/b.mp3", MetadataConfidence: "low"},
		{ID: "a", Artist: "Artist", Title: "Song", AudioPath: "/missing/a.mp3", MetadataConfidence: "low"},
	}
	first := scanLibraryHealth(tracks)
	second := scanLibraryHealth([]TrackRecord{tracks[1], tracks[0]})
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("scan should be stable regardless of input order:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if len(first.Issues) == 0 {
		t.Fatal("expected health findings")
	}
	for _, issue := range first.Issues {
		if issue.ID == "" || issue.ProposedFix == "" || len(issue.Evidence) == 0 {
			t.Fatalf("expected explainable issue: %#v", issue)
		}
	}
}

func TestScanLibraryHealthDoesNotReportExistingFilesAsMissing(t *testing.T) {
	tempDir := t.TempDir()
	audio := writeHealthTestFile(t, tempDir+"/song.mp3", "audio")
	metadata := writeHealthTestFile(t, tempDir+"/song.metadata.json", "{}")
	artwork := writeHealthTestFile(t, tempDir+"/song.jpg", "art")
	lyrics := writeHealthTestFile(t, tempDir+"/song.txt", "lyrics")
	lrc := writeHealthTestFile(t, tempDir+"/song.lrc", "[00:01.00] lyrics")
	track := TrackRecord{ID: "ready", Title: "Song", Artist: "Artist", AudioPath: audio, MetadataPath: metadata, ArtworkPath: artwork, LyricsPath: lyrics, LRCPath: lrc, MetadataConfidence: "high"}
	for _, issue := range scanLibraryHealth([]TrackRecord{track}).Issues {
		if issue.Kind == "missing-audio" || issue.Kind == "missing-metadata" || issue.Kind == "missing-artwork" || issue.Kind == "missing-lyrics" || issue.Kind == "missing-timed-lyrics" || issue.Kind == "low-metadata-confidence" {
			t.Fatalf("unexpected issue for ready track: %#v", issue)
		}
	}
}

func writeHealthTestFile(t *testing.T, path, contents string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestDuplicateHealthIssueUsesStablePairIdentity(t *testing.T) {
	duration := 200
	left := TrackRecord{ID: "left", Artist: "A", Title: "T", DurationSeconds: &duration}
	right := TrackRecord{ID: "right", Artist: "A", Title: "T", DurationSeconds: &duration}
	report := scanLibraryHealth([]TrackRecord{right, left})
	var found *LibraryHealthIssue
	for i := range report.Issues {
		if report.Issues[i].Kind == "possible-duplicate" {
			found = &report.Issues[i]
			break
		}
	}
	if found == nil || found.RelatedTrackID != "right" {
		t.Fatalf("expected stable duplicate pair, got %#v", report.Issues)
	}
}
