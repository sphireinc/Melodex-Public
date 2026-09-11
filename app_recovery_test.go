package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRecoverStartupJobsMarksRunningJobsInterrupted(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}

	now := time.Now().UTC()
	app := &App{
		store:        store,
		info:         info,
		player:       newPlaybackEngine(),
		jobCancels:   map[string]context.CancelFunc{},
		workerStates: map[int]*workerState{},
		catalog: catalogFile{
			Version: catalogSchemaVersion,
			Jobs: []Job{
				{ID: "queued", Status: "queued", Detail: "Waiting for worker", CreatedAt: now.Add(-2 * time.Minute)},
				{ID: "running", Status: "running", Detail: "Processing import", CreatedAt: now.Add(-time.Minute), StartedAt: now.Add(-time.Minute)},
				{ID: "playlist-root", Kind: "playlist", Status: "running", Detail: "This is a playlist. Processing time extended.", CreatedAt: now.Add(-3 * time.Minute), StartedAt: now.Add(-3 * time.Minute)},
				{ID: "playlist-child-queued", Kind: "url", ParentJobID: "playlist-root", Status: "queued", Detail: "Playlist item 1/2 waiting for worker", CreatedAt: now.Add(-2 * time.Minute), PlaylistIndex: 1},
				{ID: "playlist-child-running", Kind: "url", ParentJobID: "playlist-root", Status: "running", Detail: "Playlist item 2/2: Song Two", CreatedAt: now.Add(-time.Minute), StartedAt: now.Add(-time.Minute), PlaylistIndex: 2},
				{ID: "completed", Status: "completed", Detail: "Track cataloged", CreatedAt: now.Add(-4 * time.Minute), StartedAt: now.Add(-4 * time.Minute), FinishedAt: now.Add(-3 * time.Minute)},
			},
		},
	}

	recovered, interrupted, err := app.recoverStartupJobs()
	if err != nil {
		t.Fatalf("recoverStartupJobs: %v", err)
	}
	if recovered != 6 {
		t.Fatalf("expected 6 recovered jobs, got %d", recovered)
	}
	if interrupted != 4 {
		t.Fatalf("expected 4 interrupted jobs, got %d", interrupted)
	}
	if len(app.jobQueue) != 1 || app.jobQueue[0] != "queued" {
		t.Fatalf("expected only standalone queued job to remain queued, got %#v", app.jobQueue)
	}
	byID := map[string]Job{}
	for _, job := range app.jobs {
		byID[job.ID] = job
	}
	if got := byID["running"]; got.Status != "stopped" || !strings.Contains(strings.ToLower(got.Detail), "interrupted") {
		t.Fatalf("expected running job to be marked interrupted, got %+v", got)
	}
	if got := byID["playlist-root"]; got.Status != "stopped" || !strings.Contains(strings.ToLower(got.Detail), "playlist") {
		t.Fatalf("expected playlist root to be stopped with playlist detail, got %+v", got)
	}
	if got := byID["playlist-child-queued"]; got.Status != "stopped" || !strings.Contains(strings.ToLower(got.Detail), "interrupted") {
		t.Fatalf("expected queued playlist child to be stopped, got %+v", got)
	}
	if got := byID["playlist-child-running"]; got.Status != "stopped" || !strings.Contains(strings.ToLower(got.Detail), "interrupted") {
		t.Fatalf("expected running playlist child to be stopped, got %+v", got)
	}
	if got := byID["completed"]; got.Status != "completed" {
		t.Fatalf("expected completed job unchanged, got %+v", got)
	}
}

func TestRecoverStartupJobsIsIdempotentAcrossRepeatedStartup(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(root)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}

	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	app := &App{
		store:        store,
		info:         info,
		player:       newPlaybackEngine(),
		jobCancels:   map[string]context.CancelFunc{},
		workerStates: map[int]*workerState{},
		catalog: catalogFile{
			Version: catalogSchemaVersion,
			Jobs: []Job{
				{ID: "interrupted-once", Kind: "local-file", Input: "track.mp3", Status: "running", Detail: "Processing import", CreatedAt: now, StartedAt: now},
				{ID: "already-completed", Kind: "local-file", Input: "done.mp3", Status: "completed", Detail: "Track cataloged", CreatedAt: now.Add(-time.Minute), FinishedAt: now},
			},
		},
	}

	recovered, interrupted, err := app.recoverStartupJobs()
	if err != nil {
		t.Fatalf("first recoverStartupJobs: %v", err)
	}
	if recovered != 2 || interrupted != 1 {
		t.Fatalf("first recovery counts: recovered=%d interrupted=%d", recovered, interrupted)
	}
	firstJobs := append([]Job(nil), app.jobs...)
	firstQueue := append([]string(nil), app.jobQueue...)
	firstCatalogBytes, err := os.ReadFile(store.catalogPath)
	if err != nil {
		t.Fatalf("read persisted catalog after first recovery: %v", err)
	}

	recovered, interrupted, err = app.recoverStartupJobs()
	if err != nil {
		t.Fatalf("second recoverStartupJobs: %v", err)
	}
	if recovered != 2 || interrupted != 0 {
		t.Fatalf("second recovery counts: recovered=%d interrupted=%d", recovered, interrupted)
	}
	if !reflect.DeepEqual(app.jobs, firstJobs) {
		t.Fatalf("repeated recovery changed jobs:\nfirst=%+v\nsecond=%+v", firstJobs, app.jobs)
	}
	if !reflect.DeepEqual(app.jobQueue, firstQueue) {
		t.Fatalf("repeated recovery changed queue: first=%#v second=%#v", firstQueue, app.jobQueue)
	}
	secondCatalogBytes, err := os.ReadFile(store.catalogPath)
	if err != nil {
		t.Fatalf("read persisted catalog after second recovery: %v", err)
	}
	if !bytes.Equal(secondCatalogBytes, firstCatalogBytes) {
		t.Fatalf("repeated recovery rewrote the persisted catalog")
	}

	// A fresh App instance sees the stopped job as already recovered. This is
	// the restart boundary: it must not create another job or interrupt it a
	// second time.
	persistedCatalog, err := store.loadCatalog()
	if err != nil {
		t.Fatalf("load persisted catalog: %v", err)
	}
	restarted := &App{
		store:        store,
		info:         info,
		player:       newPlaybackEngine(),
		jobCancels:   map[string]context.CancelFunc{},
		workerStates: map[int]*workerState{},
		catalog:      persistedCatalog,
	}
	recovered, interrupted, err = restarted.recoverStartupJobs()
	if err != nil {
		t.Fatalf("restart recoverStartupJobs: %v", err)
	}
	if recovered != 2 || interrupted != 0 {
		t.Fatalf("restart recovery counts: recovered=%d interrupted=%d", recovered, interrupted)
	}
	seen := map[string]struct{}{}
	for _, job := range restarted.jobs {
		if _, exists := seen[job.ID]; exists {
			t.Fatalf("restart recovery duplicated job %q", job.ID)
		}
		seen[job.ID] = struct{}{}
	}
}

func TestCleanupIncomingArtifactsPreservesRecoveredJobStages(t *testing.T) {
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
		jobs: []Job{
			{ID: "keep-me", Status: "stopped"},
			{ID: "remove-me", Status: "completed"},
		},
	}

	keepDir := filepath.Join(info.IncomingDir, "keep-me")
	removeDir := filepath.Join(info.IncomingDir, "remove-me")
	staleDir := filepath.Join(info.IncomingDir, "stale")
	if err := os.MkdirAll(keepDir, 0o755); err != nil {
		t.Fatalf("mkdir keep: %v", err)
	}
	if err := os.MkdirAll(removeDir, 0o755); err != nil {
		t.Fatalf("mkdir remove: %v", err)
	}
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatalf("mkdir stale: %v", err)
	}

	if err := app.cleanupIncomingArtifacts(); err != nil {
		t.Fatalf("cleanupIncomingArtifacts: %v", err)
	}
	if !pathExists(keepDir) {
		t.Fatalf("expected recovered job stage to remain")
	}
	if pathExists(removeDir) {
		t.Fatalf("expected completed job stage to be removed")
	}
	if pathExists(staleDir) {
		t.Fatalf("expected stale stage to be removed")
	}
}
