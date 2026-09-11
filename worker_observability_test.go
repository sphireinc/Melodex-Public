package main

import (
	"context"
	"testing"
	"time"
)

func TestWorkerStatsReportsQueueCountsAndCapacityByWorkClass(t *testing.T) {
	settings := defaultStoredSettings(t.TempDir())
	settings.MaxConcurrentDownloads = 3
	settings.MaxConcurrentVideoDownloads = 2
	settings.MaxConcurrentEnrichmentRequests = 4
	settings.MaxConcurrentLyricsRequests = 5
	app := &App{
		settings: settings,
		jobs: []Job{
			{ID: "audio", Kind: "local-file", Status: "queued"},
			{ID: "video", Kind: "url", DownloadVideo: true, Status: "queued"},
			{ID: "playlist", Kind: "playlist", Status: "queued"},
			{ID: "done", Kind: "local-file", Status: "completed"},
			{ID: "orphan", Kind: "local-file", Status: "queued"},
		},
		jobQueue: []string{"audio", "video", "playlist"},
		workerStates: map[int]*workerState{
			1: {busy: true, workClass: string(workerWorkClassAudio)},
			2: {busy: true, workClass: string(workerWorkClassVideo)},
			3: {busy: false},
		},
		activeVideoDownloads:     1,
		activeEnrichmentRequests: 2,
		activeArtworkRequests:    1,
		activeLyricsRequests:     1,
	}

	stats := app.workerStatsLocked(app.jobs)
	if stats.Audio.Capacity != 3 || stats.Audio.Active != 1 || stats.Audio.Queued != 2 {
		t.Fatalf("unexpected audio stats: %+v", stats.Audio)
	}
	if stats.Video.Capacity != 2 || stats.Video.Active != 1 || stats.Video.Queued != 1 {
		t.Fatalf("unexpected video stats: %+v", stats.Video)
	}
	if stats.Enrichment.Capacity != 4 || stats.Enrichment.Active != 1 || stats.Enrichment.Queued != 2 {
		t.Fatalf("unexpected enrichment stats: %+v", stats.Enrichment)
	}
	if stats.Lyrics.Capacity != 5 || stats.Lyrics.Active != 1 || stats.Lyrics.Queued != 2 {
		t.Fatalf("unexpected lyrics stats: %+v", stats.Lyrics)
	}
	if stats.Artwork.Capacity != 4 || stats.Artwork.Active != 1 || stats.Artwork.Queued != 2 || stats.Artwork.SharedCapacityOf != "enrichment" {
		t.Fatalf("unexpected artwork stats: %+v", stats.Artwork)
	}
	if stats.ActiveWorkers != 2 || stats.MaxWorkers != 3 {
		t.Fatalf("unexpected worker totals: active=%d max=%d", stats.ActiveWorkers, stats.MaxWorkers)
	}
}

func TestWorkerStatsReportsVideoSlotBlocking(t *testing.T) {
	settings := defaultStoredSettings(t.TempDir())
	settings.MaxConcurrentVideoDownloads = 1
	app := &App{
		settings: settings,
		jobs:     []Job{{ID: "video", Kind: "url", DownloadVideo: true, Status: "queued"}},
		jobQueue: []string{"video"},
	}
	app.activeVideoDownloads = 1

	stats := app.workerStatsLocked(app.jobs)
	if !stats.Video.Blocked || stats.Video.BlockedReason != workerBlockedReasonVideoCapacity {
		t.Fatalf("expected video capacity block, got %+v", stats.Video)
	}
	if stats.BlockedReason != workerBlockedReasonVideoCapacity {
		t.Fatalf("expected safe top-level video block reason, got %q", stats.BlockedReason)
	}
	if _, ok := app.nextEligibleQueuedJobLocked(); ok {
		t.Fatal("expected a full video slot to prevent video admission")
	}
}

func TestWorkerStatsReportsThrottleStateAndSafeReason(t *testing.T) {
	settings := defaultStoredSettings(t.TempDir())
	app := &App{
		settings: settings,
		jobs: []Job{
			{ID: "audio", Kind: "local-file", Status: "queued"},
			{ID: "video", Kind: "url", DownloadVideo: true, Status: "queued"},
		},
		jobQueue:      []string{"audio", "video"},
		downloadStats: downloadStats{throttle: time.Now().UTC().Add(10 * time.Minute)},
	}

	stats := app.workerStatsLocked(app.jobs)
	if !stats.Throttle.Active || stats.Throttle.Reason != workerThrottleReasonAuthCheck {
		t.Fatalf("expected typed throttle state, got %+v", stats.Throttle)
	}
	if stats.BlockedReason != workerBlockedReasonThrottle {
		t.Fatalf("expected safe throttle blocked reason, got %q", stats.BlockedReason)
	}
	if stats.Audio.BlockedReason != workerBlockedReasonThrottle || stats.Video.BlockedReason != workerBlockedReasonThrottle {
		t.Fatalf("expected throttle reasons on affected work classes, audio=%+v video=%+v", stats.Audio, stats.Video)
	}
	if got := app.desiredWorkerCountLocked(); got != 1 {
		t.Fatalf("expected throttle to reduce worker count to one, got %d", got)
	}
}

func TestArtworkSlotReportsSeparateActiveCountAndReleasesSharedCapacity(t *testing.T) {
	app := &App{settings: defaultStoredSettings(t.TempDir())}
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- app.withArtworkSlot(context.Background(), func() error {
			_, _, artwork, _ := app.activeThroughputCounts()
			if artwork != 1 {
				return &workerObservabilityTestError{message: "artwork slot was not observable while active"}
			}
			close(started)
			<-release
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("artwork slot did not start")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_, enrichment, artwork, _ := app.activeThroughputCounts()
	if enrichment != 0 || artwork != 0 {
		t.Fatalf("expected shared artwork/enrichment capacity to release, enrichment=%d artwork=%d", enrichment, artwork)
	}
}

type workerObservabilityTestError struct {
	message string
}

func (e *workerObservabilityTestError) Error() string { return e.message }
