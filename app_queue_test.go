package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSnapshotLockedOrdersJobsByState(t *testing.T) {
	now := time.Now().UTC()
	app := &App{
		player:       newPlaybackEngine(),
		jobs:         []Job{},
		catalog:      catalogFile{Version: 1, Tracks: []TrackRecord{}, Jobs: []Job{}},
		jobCancels:   map[string]context.CancelFunc{},
		workerStates: map[int]*workerState{},
		jobQueue:     []string{},
	}
	app.jobs = []Job{
		{ID: "queued-old", Status: "queued", CreatedAt: now.Add(-4 * time.Hour)},
		{ID: "running", Status: "running", CreatedAt: now.Add(-3 * time.Hour), StartedAt: now.Add(-2 * time.Hour)},
		{ID: "completed", Status: "completed", CreatedAt: now.Add(-3 * time.Hour), FinishedAt: now.Add(-90 * time.Minute)},
		{ID: "failed", Status: "failed", CreatedAt: now.Add(-2 * time.Hour), FinishedAt: now.Add(-80 * time.Minute)},
		{ID: "queued-new", Status: "queued", CreatedAt: now.Add(-1 * time.Hour)},
	}

	state := app.snapshotLocked()
	if len(state.Jobs) != 5 {
		t.Fatalf("expected 5 jobs, got %d", len(state.Jobs))
	}
	if state.Jobs[0].ID != "running" {
		t.Fatalf("expected running job first, got %q", state.Jobs[0].ID)
	}
	if state.Jobs[1].ID != "completed" && state.Jobs[1].ID != "failed" {
		t.Fatalf("expected done jobs in the middle, got %q", state.Jobs[1].ID)
	}
	if state.Jobs[2].ID != "completed" && state.Jobs[2].ID != "failed" {
		t.Fatalf("expected done jobs in the middle, got %q", state.Jobs[2].ID)
	}
	if state.Jobs[3].ID != "queued-old" || state.Jobs[4].ID != "queued-new" {
		t.Fatalf("expected queued jobs last in age order, got %q then %q", state.Jobs[3].ID, state.Jobs[4].ID)
	}
}

func TestPauseAllJobsFreezesQueueAndLeavesQueuedItemsQueued(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}

	cancelled := false
	app := &App{
		store:        store,
		info:         info,
		player:       newPlaybackEngine(),
		jobCancels:   map[string]context.CancelFunc{"running": func() { cancelled = true }},
		workerStates: map[int]*workerState{},
		jobQueue:     []string{"queued"},
		catalog:      catalogFile{Version: 1, Tracks: []TrackRecord{}, Jobs: []Job{}},
	}
	app.jobs = []Job{
		{ID: "running", Status: "running", CreatedAt: time.Now().UTC().Add(-time.Hour)},
		{ID: "queued", Status: "queued", CreatedAt: time.Now().UTC()},
	}
	app.catalog.Jobs = append([]Job(nil), app.jobs...)

	state, err := app.PauseAllJobs()
	if err != nil {
		t.Fatalf("PauseAllJobs: %v", err)
	}
	if !app.queueFrozen {
		t.Fatalf("expected queueFrozen to be true")
	}
	if !cancelled {
		t.Fatalf("expected running job cancel to be called")
	}
	if len(app.jobQueue) != 1 || app.jobQueue[0] != "queued" {
		t.Fatalf("expected queued job to stay pending, got %#v", app.jobQueue)
	}
	if len(state.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(state.Jobs))
	}
	if state.Jobs[0].ID != "running" || state.Jobs[0].Status != "stopped" {
		t.Fatalf("expected running job to stop, got %s/%s", state.Jobs[0].ID, state.Jobs[0].Status)
	}
	if state.Jobs[1].ID != "queued" || state.Jobs[1].Status != "queued" {
		t.Fatalf("expected queued job to remain queued, got %s/%s", state.Jobs[1].ID, state.Jobs[1].Status)
	}
}

func TestDeleteAllJobsCancelsPlaylistChildren(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}

	rootCancelled := false
	childCancelled := false
	app := &App{
		store:        store,
		info:         info,
		player:       newPlaybackEngine(),
		jobCancels:   map[string]context.CancelFunc{"root": func() { rootCancelled = true }, "child": func() { childCancelled = true }},
		workerStates: map[int]*workerState{},
		jobQueue:     []string{"root", "child"},
		catalog:      catalogFile{Version: 1, Tracks: []TrackRecord{}, Jobs: []Job{}},
	}
	now := time.Now().UTC()
	app.jobs = []Job{
		{ID: "root", Kind: "playlist", Status: "running", Detail: "playlist", CreatedAt: now},
		{ID: "child", Kind: "url", ParentJobID: "root", Status: "running", Detail: "child", CreatedAt: now.Add(time.Second), PlaylistIndex: 1, PlaylistTotalItems: 2},
		{ID: "queued", Kind: "url", Status: "queued", Detail: "queued", CreatedAt: now.Add(2 * time.Second)},
	}
	app.catalog.Jobs = append([]Job(nil), app.jobs...)

	state, err := app.DeleteAllJobs()
	if err != nil {
		t.Fatalf("DeleteAllJobs: %v", err)
	}
	if !rootCancelled {
		t.Fatalf("expected playlist root cancel to be called")
	}
	if !childCancelled {
		t.Fatalf("expected playlist child cancel to be called")
	}
	if len(app.jobs) != 0 {
		t.Fatalf("expected all jobs removed, got %d", len(app.jobs))
	}
	if len(app.jobQueue) != 0 {
		t.Fatalf("expected queue to be cleared, got %#v", app.jobQueue)
	}
	if len(state.Jobs) != 0 {
		t.Fatalf("expected empty state jobs, got %d", len(state.Jobs))
	}
}

func TestProcessPlaylistImportJobExpandsAndRollsUp(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}

	app := &App{
		store:        store,
		info:         info,
		player:       newPlaybackEngine(),
		jobCancels:   map[string]context.CancelFunc{},
		workerStates: map[int]*workerState{},
		jobQueue:     []string{},
		catalog:      catalogFile{Version: 1, Tracks: []TrackRecord{}, Jobs: []Job{}},
	}
	now := time.Now().UTC()
	rootJob := Job{
		ID:        "root",
		Kind:      "url",
		Input:     "https://www.youtube.com/playlist?list=abc",
		Status:    "running",
		Detail:    "Processing import",
		CreatedAt: now,
		StartedAt: now,
	}
	app.jobs = []Job{rootJob}
	app.catalog.Jobs = append([]Job(nil), app.jobs...)

	detail, err := app.processPlaylistImportJob(rootJob, playlistDiscovery{
		Title:      "Playlist Title",
		Channel:    "Channel Name",
		WebpageURL: "https://www.youtube.com/playlist?list=abc",
		Entries: []playlistDiscoveryEntry{
			{ID: "one", Title: "Song One", URL: "https://www.youtube.com/watch?v=one", PlaylistIndex: 1},
			{ID: "two", Title: "Song Two", URL: "https://www.youtube.com/watch?v=two", PlaylistIndex: 2},
		},
	})
	if err != nil {
		t.Fatalf("processPlaylistImportJob: %v", err)
	}
	if !strings.Contains(detail, "playlist") {
		t.Fatalf("expected playlist detail, got %q", detail)
	}
	if len(app.jobs) != 3 {
		t.Fatalf("expected root + 2 children, got %d jobs", len(app.jobs))
	}
	if app.jobs[0].Kind != "playlist" || app.jobs[0].PlaylistTotalItems != 2 {
		t.Fatalf("expected playlist root to be updated, got %+v", app.jobs[0])
	}
	if app.jobs[0].PlaylistProcessedItems != 0 || app.jobs[0].PlaylistFailedItems != 0 {
		t.Fatalf("expected fresh playlist counts to start at zero, got %+v", app.jobs[0])
	}
	if app.jobs[0].PlaylistCurrentTitle != "Song One" {
		t.Fatalf("expected first queued child to be current item, got %q", app.jobs[0].PlaylistCurrentTitle)
	}
	if len(app.jobQueue) != 2 {
		t.Fatalf("expected 2 child jobs queued, got %#v", app.jobQueue)
	}
	for _, job := range app.jobs[1:] {
		if job.ParentJobID != "root" {
			t.Fatalf("expected playlist child to be linked to root, got %+v", job)
		}
	}
}

func TestRetryFailedPlaylistItemsRequeuesOnlyFailedChildren(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}

	app := &App{
		store:        store,
		info:         info,
		player:       newPlaybackEngine(),
		jobCancels:   map[string]context.CancelFunc{},
		workerStates: map[int]*workerState{},
		jobQueue:     []string{},
		catalog:      catalogFile{Version: 1, Tracks: []TrackRecord{}, Jobs: []Job{}},
	}
	now := time.Now().UTC()
	app.jobs = []Job{
		{
			ID:                     "root",
			Kind:                   "playlist",
			Status:                 "completed",
			Detail:                 "Playlist import complete: 1/2 succeeded, 1 failed.",
			CreatedAt:              now,
			StartedAt:              now.Add(-time.Minute),
			FinishedAt:             now,
			PlaylistURL:            "https://www.youtube.com/playlist?list=abc",
			PlaylistTitle:          "Playlist Title",
			PlaylistChannel:        "Channel Name",
			PlaylistTotalItems:     3,
			PlaylistProcessedItems: 2,
			PlaylistFailedItems:    1,
			PlaylistCurrentIndex:   2,
			PlaylistCurrentTitle:   "Song Two",
		},
		{
			ID:                 "ok",
			Kind:               "url",
			ParentJobID:        "root",
			Status:             "completed",
			Detail:             "Track cataloged",
			CreatedAt:          now,
			StartedAt:          now.Add(-time.Minute),
			FinishedAt:         now,
			PlaylistTitle:      "Playlist Title",
			PlaylistTotalItems: 3,
			PlaylistIndex:      1,
			PlaylistItemTitle:  "Song One",
		},
		{
			ID:                 "bad",
			Kind:               "url",
			ParentJobID:        "root",
			Status:             "failed",
			Detail:             "Import failed: yt-dlp requires authentication. Set a cookies file or cookies-from-browser in Settings > Downloader, then retry: yt-dlp failed: sign in to confirm you’re not a bot",
			Error:              "yt-dlp requires authentication. Set a cookies file or cookies-from-browser in Settings > Downloader, then retry: yt-dlp failed: sign in to confirm you’re not a bot",
			CreatedAt:          now.Add(time.Second),
			StartedAt:          now.Add(-30 * time.Second),
			FinishedAt:         now,
			PlaylistTitle:      "Playlist Title",
			PlaylistTotalItems: 3,
			PlaylistIndex:      2,
			PlaylistItemTitle:  "Song Two",
		},
		{
			ID:                 "later",
			Kind:               "url",
			ParentJobID:        "root",
			Status:             "queued",
			Detail:             "Waiting for worker",
			CreatedAt:          now.Add(2 * time.Second),
			PlaylistTitle:      "Playlist Title",
			PlaylistTotalItems: 3,
			PlaylistIndex:      3,
			PlaylistItemTitle:  "Song Three",
		},
	}
	app.catalog.Jobs = append([]Job(nil), app.jobs...)
	app.jobQueue = []string{"later"}

	state, err := app.RetryFailedPlaylistItems("root")
	if err != nil {
		t.Fatalf("RetryFailedPlaylistItems: %v", err)
	}
	if len(app.jobQueue) != 2 || app.jobQueue[0] != "bad" || app.jobQueue[1] != "later" {
		t.Fatalf("expected failed child to be requeued, got %#v", app.jobQueue)
	}
	if len(app.jobs) != 4 {
		t.Fatalf("expected 4 jobs to remain, got %d", len(app.jobs))
	}
	if app.jobs[0].Status != "running" {
		t.Fatalf("expected root to be running after retry, got %s", app.jobs[0].Status)
	}
	if app.jobs[0].PlaylistFailedItems != 0 {
		t.Fatalf("expected root failed count to clear after retry, got %d", app.jobs[0].PlaylistFailedItems)
	}
	if app.jobs[2].Status != "queued" || app.jobs[2].Detail != "Waiting for worker" || app.jobs[2].Error != "" {
		t.Fatalf("expected failed child to reset for retry, got %+v", app.jobs[2])
	}
	var queued Job
	for _, job := range state.Jobs {
		if job.ID == "bad" {
			queued = job
			break
		}
	}
	if queued.ID == "" || queued.Status != "queued" {
		t.Fatalf("expected queued child in snapshot, got %+v", queued)
	}
}

func TestFailJobBubblesDetailedError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}

	app := &App{
		store:        store,
		info:         info,
		player:       newPlaybackEngine(),
		jobCancels:   map[string]context.CancelFunc{},
		workerStates: map[int]*workerState{},
		jobQueue:     []string{},
		catalog:      catalogFile{Version: 1, Tracks: []TrackRecord{}, Jobs: []Job{}},
	}
	app.jobs = []Job{
		{ID: "job", Kind: "url", Status: "running", Detail: "Processing import", CreatedAt: time.Now().UTC()},
	}
	app.catalog.Jobs = append([]Job(nil), app.jobs...)

	err = errors.New("yt-dlp requires authentication. Set a cookies file or cookies-from-browser in Settings > Downloader, then retry")
	app.failJob("job", err)

	if !strings.Contains(app.jobs[0].Detail, "yt-dlp requires authentication") {
		t.Fatalf("expected job detail to include auth error, got %q", app.jobs[0].Detail)
	}
	if app.jobs[0].Error != err.Error() {
		t.Fatalf("expected job error to match input error, got %q", app.jobs[0].Error)
	}
}

func TestRetryFailedPlaylistItemsNoopWhenNothingFailed(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}

	app := &App{
		store:        store,
		info:         info,
		player:       newPlaybackEngine(),
		jobCancels:   map[string]context.CancelFunc{},
		workerStates: map[int]*workerState{},
		jobQueue:     []string{"queued"},
		catalog:      catalogFile{Version: 1, Tracks: []TrackRecord{}, Jobs: []Job{}},
	}
	now := time.Now().UTC()
	app.jobs = []Job{
		{
			ID:                     "root",
			Kind:                   "playlist",
			Status:                 "completed",
			Detail:                 "Playlist import complete: 1/1 items imported.",
			CreatedAt:              now,
			StartedAt:              now.Add(-time.Minute),
			FinishedAt:             now,
			PlaylistURL:            "https://www.youtube.com/playlist?list=abc",
			PlaylistTitle:          "Playlist Title",
			PlaylistChannel:        "Channel Name",
			PlaylistTotalItems:     1,
			PlaylistProcessedItems: 1,
			PlaylistFailedItems:    0,
		},
		{
			ID:                 "queued",
			Kind:               "url",
			ParentJobID:        "root",
			Status:             "queued",
			Detail:             "Waiting for worker",
			CreatedAt:          now.Add(time.Second),
			PlaylistTitle:      "Playlist Title",
			PlaylistTotalItems: 1,
			PlaylistIndex:      1,
			PlaylistItemTitle:  "Song One",
		},
	}
	app.catalog.Jobs = append([]Job(nil), app.jobs...)

	state, err := app.RetryFailedPlaylistItems("root")
	if err != nil {
		t.Fatalf("RetryFailedPlaylistItems: %v", err)
	}
	if len(app.jobQueue) != 1 || app.jobQueue[0] != "queued" {
		t.Fatalf("expected queue to remain unchanged, got %#v", app.jobQueue)
	}
	if len(state.Jobs) != 2 {
		t.Fatalf("expected snapshot to remain unchanged, got %d jobs", len(state.Jobs))
	}
	if app.jobs[0].Status != "completed" || app.jobs[0].PlaylistFailedItems != 0 {
		t.Fatalf("expected root to remain completed with zero failed items, got %+v", app.jobs[0])
	}
}
