package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDesiredWorkerCountLockedScalesAndThrottles(t *testing.T) {
	root := t.TempDir()
	baseSettings := defaultStoredSettings(root)

	tests := []struct {
		name        string
		settings    storedSettings
		queueFrozen bool
		throttle    time.Time
		jobs        []Job
		durations   []time.Duration
		want        int
	}{
		{
			name:     "idle defaults to one worker",
			settings: baseSettings,
			want:     1,
		},
		{
			name:        "queue frozen stays at one",
			settings:    baseSettings,
			queueFrozen: true,
			want:        1,
		},
		{
			name:     "auth throttle forces one",
			settings: baseSettings,
			throttle: time.Now().Add(10 * time.Minute),
			want:     1,
		},
		{
			name: "large backlog and healthy downloads ramps to three",
			settings: func() storedSettings {
				next := baseSettings
				next.MaxConcurrentDownloads = 3
				return next
			}(),
			jobs: queuedJobs(100),
			durations: []time.Duration{
				5 * time.Minute,
				6 * time.Minute,
				7 * time.Minute,
			},
			want: 3,
		},
		{
			name:     "medium backlog and healthy downloads ramps to two",
			settings: baseSettings,
			jobs:     queuedJobs(20),
			durations: []time.Duration{
				5 * time.Minute,
				6 * time.Minute,
				7 * time.Minute,
			},
			want: 2,
		},
		{
			name:     "slow downloads stay at one",
			settings: baseSettings,
			jobs:     queuedJobs(50),
			durations: []time.Duration{
				16 * time.Minute,
				17 * time.Minute,
			},
			want: 1,
		},
		{
			name: "max concurrent downloads caps the pool",
			settings: func() storedSettings {
				next := baseSettings
				next.MaxConcurrentDownloads = 2
				return next
			}(),
			jobs:      queuedJobs(100),
			durations: []time.Duration{5 * time.Minute},
			want:      2,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			app := &App{
				settings: tc.settings,
				jobs:     append([]Job(nil), tc.jobs...),
			}
			app.downloadStats.durations = append([]time.Duration(nil), tc.durations...)
			app.downloadStats.throttle = tc.throttle
			app.queueFrozen = tc.queueFrozen

			if got := app.desiredWorkerCountLocked(); got != tc.want {
				t.Fatalf("desiredWorkerCountLocked() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestNextEligibleQueuedJobLockedSkipsVideoJobsWhenSlotsAreFull(t *testing.T) {
	app := &App{
		settings: defaultStoredSettings(t.TempDir()),
		jobs: []Job{
			{ID: "video", Status: "queued", DownloadVideo: true},
			{ID: "audio", Status: "queued"},
		},
		jobQueue: []string{"video", "audio"},
	}
	app.throughputMu.Lock()
	app.activeVideoDownloads = 1
	app.throughputMu.Unlock()
	app.settings.MaxConcurrentVideoDownloads = 1

	got, ok := app.nextEligibleQueuedJobLocked()
	if !ok {
		t.Fatalf("expected queued job to be selected")
	}
	if got != "audio" {
		t.Fatalf("expected audio job to be selected, got %q", got)
	}
	if len(app.jobQueue) != 1 || app.jobQueue[0] != "video" {
		t.Fatalf("expected video job to remain queued, got %#v", app.jobQueue)
	}
}

func TestWorkerPoolReconcilesAfterConcurrencySettingChange(t *testing.T) {
	settings := defaultStoredSettings(t.TempDir())
	settings.MaxConcurrentDownloads = 3
	settings.MaxConcurrentVideoDownloads = 1
	settings.VideoDownloadMode = videoDownloadModeOnDemand

	jobs := queuedJobs(100)
	jobQueue := make([]string, 0, len(jobs))
	for i := range jobs {
		jobs[i].ID = "job-" + string(rune('a'+i%26)) + "-" + time.Unix(int64(i), 0).Format("150405.000000000")
		jobQueue = append(jobQueue, jobs[i].ID)
	}

	cancelled := make(map[int]int)
	app := &App{
		settings:     settings,
		jobs:         jobs,
		jobQueue:     jobQueue,
		workerAdjust: make(chan struct{}, 1),
		workerStates: map[int]*workerState{
			1: {cancel: func() { cancelled[1]++ }},
			2: {cancel: func() { cancelled[2]++ }},
			3: {busy: true, workClass: string(workerWorkClassAudio), cancel: func() { cancelled[3]++ }},
		},
		downloadStats: downloadStats{durations: []time.Duration{5 * time.Minute}},
	}

	// This is the same notification path used by SaveSettings. Keep the
	// manager out of the test so worker admission and fake cancellation stay
	// deterministic.
	app.settings.MaxConcurrentDownloads = 1
	app.requestWorkerPoolReconcile()
	select {
	case <-app.workerAdjust:
	default:
		t.Fatal("settings change did not request worker-pool reconciliation")
	}

	app.reconcileWorkerPool(context.Background())
	if got := len(app.workerStates); got != 1 {
		t.Fatalf("expected one worker after lowering the limit, got %d", got)
	}
	if _, ok := app.workerStates[3]; !ok {
		t.Fatal("expected the busy worker to remain until its current job finishes")
	}
	if cancelled[1]+cancelled[2] != 2 || cancelled[3] != 0 {
		t.Fatalf("expected only idle workers to be cancelled, got cancellations=%v", cancelled)
	}
	if app.settings.MaxConcurrentVideoDownloads != 1 || app.settings.VideoDownloadMode != videoDownloadModeOnDemand {
		t.Fatalf("worker reconciliation changed video policy: videoLimit=%d mode=%q", app.settings.MaxConcurrentVideoDownloads, app.settings.VideoDownloadMode)
	}

	// Once the busy job releases its worker, another reconciliation must not
	// cancel or replace the remaining worker merely because the setting changed.
	app.workerStates[3].busy = false
	app.reconcileWorkerPool(context.Background())
	if got := len(app.workerStates); got != 1 || cancelled[3] != 0 {
		t.Fatalf("expected the settled pool to remain stable, workers=%d cancellations=%v", got, cancelled)
	}
}

func TestPerJobVideoSlotDeadlineAndCancellationAreIsolated(t *testing.T) {
	app := &App{settings: defaultStoredSettings(t.TempDir())}
	app.settings.MaxConcurrentVideoDownloads = 1

	firstStarted := make(chan struct{})
	firstDone := make(chan error, 1)
	firstContext, cancelFirst := context.WithCancel(context.Background())
	go func() {
		firstDone <- app.withVideoDownloadSlot(firstContext, func() error {
			close(firstStarted)
			<-firstContext.Done()
			return firstContext.Err()
		})
	}()
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first job did not acquire the video slot")
	}

	secondContext, cancelSecond := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	secondCallbackCalled := false
	go func() {
		secondDone <- app.withVideoDownloadSlot(secondContext, func() error {
			secondCallbackCalled = true
			return nil
		})
	}()
	cancelSecond()
	select {
	case err := <-secondDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting job returned %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled waiting job did not return")
	}
	if secondCallbackCalled {
		t.Fatal("cancelled waiting job unexpectedly entered its callback")
	}
	if video, _, _, _ := app.activeThroughputCounts(); video != 1 {
		t.Fatalf("cancelled waiting job disturbed the active job's slot: active video=%d", video)
	}

	deadlineContext, cancelDeadline := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelDeadline()
	deadlineCallbackCalled := false
	err := app.withVideoDownloadSlot(deadlineContext, func() error {
		deadlineCallbackCalled = true
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline-limited waiting job returned %v, want context.DeadlineExceeded", err)
	}
	if deadlineCallbackCalled {
		t.Fatal("deadline-limited waiting job unexpectedly entered its callback")
	}
	if video, _, _, _ := app.activeThroughputCounts(); video != 1 {
		t.Fatalf("deadline-limited waiting job disturbed the active job's slot: active video=%d", video)
	}

	cancelFirst()
	select {
	case err := <-firstDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("active job returned %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled active job did not finish")
	}
	if video, _, _, _ := app.activeThroughputCounts(); video != 0 {
		t.Fatalf("active job did not release its video slot after cancellation: active video=%d", video)
	}

	admitted := false
	if err := app.withVideoDownloadSlot(context.Background(), func() error {
		admitted = true
		return nil
	}); err != nil {
		t.Fatalf("new job after cancellation: %v", err)
	}
	if !admitted {
		t.Fatal("new job was not admitted after the cancelled job released its slot")
	}
}

func queuedJobs(count int) []Job {
	jobs := make([]Job, 0, count)
	now := time.Now().UTC()
	for i := 0; i < count; i++ {
		jobs = append(jobs, Job{
			ID:        "job",
			Status:    "queued",
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		})
	}
	return jobs
}
