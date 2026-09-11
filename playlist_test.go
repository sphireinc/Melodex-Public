package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newPlaylistTestApp(t *testing.T) *App {
	t.Helper()
	store, info, err := newStore(filepath.Join(t.TempDir(), "Music"))
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	return &App{
		store:        store,
		info:         info,
		player:       newPlaybackEngine(),
		jobCancels:   map[string]context.CancelFunc{},
		workerStates: map[int]*workerState{},
		jobQueue:     []string{},
		catalog:      catalogFile{Version: catalogSchemaVersion, Tracks: []TrackRecord{}, Jobs: []Job{}},
	}
}

func writePlaylistDiscoveryFixture(t *testing.T, payload string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "yt-dlp-fixture")
	script := "#!/bin/sh\ncat <<'MELODEX_JSON'\n" + payload + "\nMELODEX_JSON\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write yt-dlp fixture: %v", err)
	}
	return path
}

func TestDiscoverPlaylistRecognizesSingleEntryAndFallbackIndex(t *testing.T) {
	app := newPlaylistTestApp(t)
	ytdlp := writePlaylistDiscoveryFixture(t, `{"_type":"playlist","title":"One Track Playlist","webpage_url":"https://www.youtube.com/playlist?list=one","entries":[{"id":"one","title":"Song One","webpage_url":"https://www.youtube.com/watch?v=one","playlist_index":0}]}`)

	discovery, isPlaylist, err := app.discoverPlaylist(context.Background(), ytdlp, "https://www.youtube.com/playlist?list=one")
	if err != nil {
		t.Fatalf("discoverPlaylist: %v", err)
	}
	if !isPlaylist {
		t.Fatal("expected a one-entry playlist to remain a playlist batch")
	}
	if len(discovery.Entries) != 1 {
		t.Fatalf("expected one discovered entry, got %d", len(discovery.Entries))
	}
	if discovery.Entries[0].PlaylistIndex != 1 {
		t.Fatalf("expected missing playlist index to fall back to position 1, got %d", discovery.Entries[0].PlaylistIndex)
	}
	if discovery.Entries[0].URL != "https://www.youtube.com/watch?v=one" {
		t.Fatalf("unexpected discovered entry URL %q", discovery.Entries[0].URL)
	}
}

func TestDiscoverPlaylistDoesNotExpandSingleTrack(t *testing.T) {
	app := newPlaylistTestApp(t)
	ytdlp := writePlaylistDiscoveryFixture(t, `{"_type":"video","title":"Single Track","webpage_url":"https://www.youtube.com/watch?v=one"}`)

	discovery, isPlaylist, err := app.discoverPlaylist(context.Background(), ytdlp, "https://www.youtube.com/watch?v=one")
	if err != nil {
		t.Fatalf("discoverPlaylist: %v", err)
	}
	if isPlaylist {
		t.Fatalf("expected single-track discovery not to expand: %+v", discovery)
	}
	if len(discovery.Entries) != 0 {
		t.Fatalf("expected no playlist entries for a single track, got %d", len(discovery.Entries))
	}
}

func TestRefreshPlaylistBatchRollsUpPartialFailureAndCompletion(t *testing.T) {
	app := newPlaylistTestApp(t)
	now := time.Now().UTC()
	app.jobs = []Job{
		{ID: "root", Kind: "playlist", Status: "running", PlaylistTitle: "Batch", CreatedAt: now},
		{ID: "one", Kind: "url", ParentJobID: "root", Status: "completed", PlaylistIndex: 1, PlaylistItemTitle: "One", CreatedAt: now},
		{ID: "two", Kind: "url", ParentJobID: "root", Status: "failed", PlaylistIndex: 2, PlaylistItemTitle: "Two", CreatedAt: now.Add(time.Second)},
		{ID: "three", Kind: "url", ParentJobID: "root", Status: "queued", PlaylistIndex: 3, PlaylistItemTitle: "Three", CreatedAt: now.Add(2 * time.Second)},
	}
	app.catalog.Jobs = append([]Job(nil), app.jobs...)

	app.mu.Lock()
	completed := app.refreshPlaylistBatchLocked("root")
	root := app.jobs[0]
	app.mu.Unlock()
	if completed {
		t.Fatal("expected a playlist with queued work to remain active")
	}
	if root.PlaylistTotalItems != 3 || root.PlaylistProcessedItems != 2 || root.PlaylistFailedItems != 1 {
		t.Fatalf("unexpected partial roll-up: %+v", root)
	}
	if root.PlaylistCurrentIndex != 3 || root.PlaylistCurrentTitle != "Three" {
		t.Fatalf("expected queued child to be current, got index=%d title=%q", root.PlaylistCurrentIndex, root.PlaylistCurrentTitle)
	}
	if !strings.Contains(root.Detail, "Processing item 3/3") {
		t.Fatalf("expected active playlist detail, got %q", root.Detail)
	}

	app.mu.Lock()
	app.jobs[3].Status = "completed"
	completed = app.refreshPlaylistBatchLocked("root")
	root = app.jobs[0]
	app.mu.Unlock()
	if !completed || root.Status != "completed" {
		t.Fatalf("expected playlist to complete after final child, completed=%t root=%+v", completed, root)
	}
	if root.PlaylistProcessedItems != 3 || root.PlaylistFailedItems != 1 {
		t.Fatalf("expected completed roll-up to preserve failed count, got %+v", root)
	}
	if !strings.Contains(root.Detail, "1 failed") {
		t.Fatalf("expected partial-success warning in detail, got %q", root.Detail)
	}
}

func TestEmptyPlaylistCompletesWithoutStuckRoot(t *testing.T) {
	app := newPlaylistTestApp(t)
	now := time.Now().UTC()
	root := Job{ID: "root", Kind: "url", Input: "https://www.youtube.com/playlist?list=empty", Status: "running", CreatedAt: now}
	app.jobs = []Job{root}
	app.catalog.Jobs = append([]Job(nil), app.jobs...)

	if _, err := app.processPlaylistImportJob(root, playlistDiscovery{
		Title:      "Empty Playlist",
		WebpageURL: root.Input,
		Entries:    []playlistDiscoveryEntry{},
	}); err != nil {
		t.Fatalf("processPlaylistImportJob: %v", err)
	}
	if len(app.jobs) != 1 || len(app.jobQueue) != 0 {
		t.Fatalf("expected only completed root and no queued children, jobs=%d queue=%#v", len(app.jobs), app.jobQueue)
	}
	if app.jobs[0].Status != "completed" {
		t.Fatalf("expected empty playlist root to complete, got %+v", app.jobs[0])
	}
	if !strings.Contains(app.jobs[0].Detail, "no importable items") {
		t.Fatalf("expected empty playlist explanation, got %q", app.jobs[0].Detail)
	}
}

func TestStopPlaylistCancelsChildrenAndPreservesBatchIdentity(t *testing.T) {
	app := newPlaylistTestApp(t)
	now := time.Now().UTC()
	rootCancelCalled := false
	childCancelCalled := false
	app.jobCancels["root"] = func() { rootCancelCalled = true }
	app.jobCancels["child-running"] = func() { childCancelCalled = true }
	app.jobs = []Job{
		{ID: "root", Kind: "playlist", Status: "running", PlaylistURL: "https://www.youtube.com/playlist?list=abc", PlaylistTitle: "Batch", CreatedAt: now},
		{ID: "child-running", Kind: "url", ParentJobID: "root", Status: "running", PlaylistIndex: 1, PlaylistItemTitle: "Running", CreatedAt: now},
		{ID: "child-queued", Kind: "url", ParentJobID: "root", Status: "queued", PlaylistIndex: 2, PlaylistItemTitle: "Queued", CreatedAt: now.Add(time.Second)},
	}
	app.catalog.Jobs = append([]Job(nil), app.jobs...)
	app.jobQueue = []string{"child-queued"}

	state, err := app.StopJob("root")
	if err != nil {
		t.Fatalf("StopJob: %v", err)
	}
	if !rootCancelCalled || !childCancelCalled {
		t.Fatalf("expected root and running child cancellation, root=%t child=%t", rootCancelCalled, childCancelCalled)
	}
	if len(app.jobQueue) != 0 {
		t.Fatalf("expected queued children removed on stop, got %#v", app.jobQueue)
	}
	if len(state.Jobs) != 3 {
		t.Fatalf("expected root and children to remain in state, got %d jobs", len(state.Jobs))
	}
	for _, job := range state.Jobs {
		if job.Status != "stopped" {
			t.Fatalf("expected stopped playlist job, got %+v", job)
		}
		if job.ID != "root" && job.ParentJobID != "root" {
			t.Fatalf("expected child parent identity to remain intact, got %+v", job)
		}
	}
	if state.Jobs[0].PlaylistURL != "https://www.youtube.com/playlist?list=abc" || state.Jobs[0].PlaylistTitle != "Batch" {
		t.Fatalf("expected playlist identity to remain intact, got %+v", state.Jobs[0])
	}
}

func TestRetryFailedPlaylistItemsIsIdempotent(t *testing.T) {
	app := newPlaylistTestApp(t)
	now := time.Now().UTC()
	app.jobs = []Job{
		{ID: "root", Kind: "playlist", Status: "completed", PlaylistTotalItems: 2, CreatedAt: now},
		{ID: "failed", Kind: "url", ParentJobID: "root", Status: "failed", PlaylistIndex: 1, PlaylistItemTitle: "Failed", CreatedAt: now},
		{ID: "done", Kind: "url", ParentJobID: "root", Status: "completed", PlaylistIndex: 2, PlaylistItemTitle: "Done", CreatedAt: now.Add(time.Second)},
	}
	app.catalog.Jobs = append([]Job(nil), app.jobs...)

	if _, err := app.RetryFailedPlaylistItems("root"); err != nil {
		t.Fatalf("first retry: %v", err)
	}
	if len(app.jobQueue) != 1 || app.jobQueue[0] != "failed" {
		t.Fatalf("expected failed child queued once, got %#v", app.jobQueue)
	}
	if _, err := app.RetryFailedPlaylistItems("root"); err != nil {
		t.Fatalf("second retry: %v", err)
	}
	if len(app.jobQueue) != 1 || app.jobQueue[0] != "failed" {
		t.Fatalf("expected repeated retry not to duplicate queue entry, got %#v", app.jobQueue)
	}
	if app.jobs[2].Status != "completed" {
		t.Fatalf("expected completed child to remain untouched, got %+v", app.jobs[2])
	}
}
