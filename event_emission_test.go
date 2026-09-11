package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var eventTestLogMu sync.Mutex

func newEventTestApp(t testing.TB) *application.App {
	t.Helper()
	return application.New(application.Options{
		DisableDefaultSignalHandler: true,
	})
}

func captureEventInstrumentation(t *testing.T) *bytes.Buffer {
	t.Helper()
	eventTestLogMu.Lock()
	t.Cleanup(eventTestLogMu.Unlock)

	previousWriter := log.Writer()
	var output bytes.Buffer
	log.SetOutput(&output)
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
	})
	t.Setenv("MELODEX_DEBUG", "true")
	return &output
}

func TestEmitStateEventLogsSerializedPayloadSizeWithoutPayload(t *testing.T) {
	app := &App{wailsApp: newEventTestApp(t)}
	output := captureEventInstrumentation(t)

	state := AppState{
		Jobs: []Job{{ID: "job-1", Detail: strings.Repeat("detail-", 128)}},
		LibraryTracks: []TrackRecord{{
			ID:    "track-1",
			Title: strings.Repeat("track-title-", 64),
		}},
	}
	serialized, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("json.Marshal state: %v", err)
	}

	app.emitStateLocked(state)

	logText := output.String()
	expectedSize := fmt.Sprintf("payload_bytes=%q", strconv.Itoa(len(serialized)))
	if !strings.Contains(logText, "event=state_event_emitted") {
		t.Fatalf("expected state event instrumentation, got %q", logText)
	}
	if !strings.Contains(logText, expectedSize) {
		t.Fatalf("expected serialized state size %s, got %q", expectedSize, logText)
	}
	if strings.Contains(logText, state.Jobs[0].Detail) || strings.Contains(logText, state.LibraryTracks[0].Title) {
		t.Fatalf("state event instrumentation must not include the full payload: %q", logText)
	}
}

func TestEmitPlaybackEventLogsSerializedPayloadSize(t *testing.T) {
	app := &App{wailsApp: newEventTestApp(t)}
	output := captureEventInstrumentation(t)
	state := PlaybackState{
		CurrentTrackID: "track-1",
		Queue:          []string{"track-1", "track-2"},
		QueueSource:    "library",
		IsPlaying:      true,
		Volume:         0.75,
	}
	serialized, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("json.Marshal playback state: %v", err)
	}

	app.emitPlaybackState(state)

	logText := output.String()
	if !strings.Contains(logText, "event=playback_event_emitted") {
		t.Fatalf("expected playback event instrumentation, got %q", logText)
	}
	if !strings.Contains(logText, fmt.Sprintf("payload_bytes=%q", strconv.Itoa(len(serialized)))) {
		t.Fatalf("expected serialized playback size %d, got %q", len(serialized), logText)
	}
}

func TestEmitJobProgressEventDeliversOnlyTargetedProgressPayload(t *testing.T) {
	wailsApp := newEventTestApp(t)
	app := &App{wailsApp: wailsApp}
	received := make(chan any, 1)
	removeListener := wailsApp.Event.On("melodex:job-progress", func(event *application.CustomEvent) {
		received <- event.Data
	})
	t.Cleanup(removeListener)

	job := Job{
		ID:                     "job-42",
		ParentJobID:            "playlist-1",
		DownloadProgress:       35,
		MetadataProgress:       60,
		LyricsProgress:         80,
		Status:                 "running",
		Detail:                 "lyrics lookup",
		PlaylistTotalItems:     4,
		PlaylistProcessedItems: 2,
		PlaylistFailedItems:    1,
		StageStatuses: JobStageStatuses{
			Download: "Downloaded",
			Lyrics:   "In progress",
		},
	}
	app.emitJobProgressLocked(job)

	select {
	case data := <-received:
		progress, ok := data.(JobProgressEvent)
		if !ok {
			t.Fatalf("expected JobProgressEvent, got %T", data)
		}
		if progress.JobID != job.ID || progress.ParentJobID != job.ParentJobID {
			t.Fatalf("expected job identity to be preserved, got %+v", progress)
		}
		if progress.DownloadProgress != job.DownloadProgress || progress.MetadataProgress != job.MetadataProgress || progress.LyricsProgress != job.LyricsProgress {
			t.Fatalf("expected progress values to be preserved, got %+v", progress)
		}
		if progress.Status != job.Status || progress.Detail != job.Detail || progress.StageStatuses != job.StageStatuses {
			t.Fatalf("expected status fields to be preserved, got %+v", progress)
		}
		if progress.PlaylistTotal != job.PlaylistTotalItems || progress.PlaylistProcessed != job.PlaylistProcessedItems || progress.PlaylistFailed != job.PlaylistFailedItems {
			t.Fatalf("expected playlist counters to be preserved, got %+v", progress)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for targeted job-progress event")
	}
}

func TestEmitPlaybackEventDeliversTypedState(t *testing.T) {
	wailsApp := newEventTestApp(t)
	app := &App{wailsApp: wailsApp}
	received := make(chan any, 1)
	removeListener := wailsApp.Event.On("melodex:playback", func(event *application.CustomEvent) {
		received <- event.Data
	})
	t.Cleanup(removeListener)

	state := PlaybackState{
		CurrentTrackID: "track-7",
		Queue:          []string{"track-7", "track-8"},
		IsPlaying:      true,
		CurrentTime:    12.5,
		Duration:       180,
		RepeatMode:     "all",
	}
	app.emitPlaybackState(state)

	select {
	case data := <-received:
		playback, ok := data.(PlaybackState)
		if !ok {
			t.Fatalf("expected PlaybackState, got %T", data)
		}
		if playback.CurrentTrackID != state.CurrentTrackID || playback.IsPlaying != state.IsPlaying || playback.CurrentTime != state.CurrentTime || playback.Duration != state.Duration || playback.RepeatMode != state.RepeatMode {
			t.Fatalf("expected playback state to be preserved, got %+v", playback)
		}
		if len(playback.Queue) != len(state.Queue) || playback.Queue[0] != state.Queue[0] || playback.Queue[1] != state.Queue[1] {
			t.Fatalf("expected playback queue to be preserved, got %+v", playback.Queue)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for targeted playback event")
	}
}

func TestEmitJobProgressEventsStayBoundedAtBulkImportScale(t *testing.T) {
	app := &App{wailsApp: newEventTestApp(t)}
	output := captureEventInstrumentation(t)

	const eventCount = 1000
	const maxPayloadBytes = 8 * 1024
	for index := 0; index < eventCount; index++ {
		app.emitJobProgressLocked(Job{
			ID:                     fmt.Sprintf("job-bulk-%d", index),
			ParentJobID:            "playlist-root",
			Status:                 "running",
			Detail:                 strings.Repeat("metadata enrichment", 32),
			StageStatuses:          JobStageStatuses{Download: "Downloaded", Metadata: "In progress", Lyrics: "Queued"},
			PlaylistTotalItems:     eventCount,
			PlaylistProcessedItems: index,
			PlaylistFailedItems:    3,
		})
	}

	emitted := 0
	maxPayload := 0
	for _, line := range strings.Split(output.String(), "\n") {
		if !strings.Contains(line, "event=job_progress_event_emitted") {
			continue
		}
		emitted++
		const marker = `payload_bytes="`
		start := strings.Index(line, marker)
		if start < 0 {
			t.Fatalf("bulk progress event is missing payload size: %q", line)
		}
		start += len(marker)
		end := strings.IndexByte(line[start:], '"')
		if end < 0 {
			t.Fatalf("bulk progress event has malformed payload size: %q", line)
		}
		payloadBytes, err := strconv.Atoi(line[start : start+end])
		if err != nil {
			t.Fatalf("parse bulk progress payload size from %q: %v", line, err)
		}
		if payloadBytes > maxPayload {
			maxPayload = payloadBytes
		}
		if payloadBytes > maxPayloadBytes {
			t.Fatalf("bulk progress event payload is %d bytes, want <= %d", payloadBytes, maxPayloadBytes)
		}
	}
	if emitted != eventCount {
		t.Fatalf("expected %d targeted progress emissions, got %d", eventCount, emitted)
	}
	if maxPayload == 0 {
		t.Fatal("expected at least one measured progress payload")
	}
}

func syntheticParentChildJobProgressEvents() []Job {
	const parentJobID = "synthetic-playlist-parent"
	jobs := []Job{{
		ID:                     parentJobID,
		Kind:                   "playlist",
		Status:                 "running",
		Detail:                 "synthetic-parent-payload-content-must-not-be-logged",
		PlaylistTotalItems:     4,
		PlaylistProcessedItems: 0,
		PlaylistFailedItems:    0,
		StageStatuses:          JobStageStatuses{Download: "synthetic-parent-stage"},
	}}

	for childIndex := 1; childIndex <= 4; childIndex++ {
		childID := fmt.Sprintf("synthetic-child-%02d", childIndex)
		jobs = append(jobs,
			Job{
				ID:               childID,
				Kind:             "url",
				ParentJobID:      parentJobID,
				Status:           "running",
				Detail:           fmt.Sprintf("synthetic-child-%02d-download-payload-content-must-not-be-logged", childIndex),
				DownloadProgress: 25,
				StageStatuses:    JobStageStatuses{Download: "synthetic-download-stage"},
			},
			Job{
				ID:               childID,
				Kind:             "url",
				ParentJobID:      parentJobID,
				Status:           "running",
				Detail:           fmt.Sprintf("synthetic-child-%02d-metadata-payload-content-must-not-be-logged", childIndex),
				DownloadProgress: 100,
				MetadataProgress: 50,
				StageStatuses:    JobStageStatuses{Download: "synthetic-download-complete", Metadata: "synthetic-metadata-stage"},
			},
			Job{
				ID:                     childID,
				Kind:                   "url",
				ParentJobID:            parentJobID,
				Status:                 "completed",
				Detail:                 fmt.Sprintf("synthetic-child-%02d-complete-payload-content-must-not-be-logged", childIndex),
				DownloadProgress:       100,
				MetadataProgress:       100,
				LyricsProgress:         100,
				PlaylistTotalItems:     4,
				PlaylistProcessedItems: childIndex,
				StageStatuses:          JobStageStatuses{Download: "synthetic-download-complete", Metadata: "synthetic-metadata-complete", Lyrics: "synthetic-lyrics-complete"},
			},
		)
	}
	return jobs
}

func TestSyntheticParentChildJobProgressEventInstrumentation(t *testing.T) {
	const (
		expectedEventCount       = 13
		expectedPayloadByteCount = 4332
	)
	wailsApp := newEventTestApp(t)
	app := &App{wailsApp: wailsApp}
	output := captureEventInstrumentation(t)
	events := syntheticParentChildJobProgressEvents()
	if len(events) != expectedEventCount {
		t.Fatalf("expected synthetic fixture to contain %d events, got %d", expectedEventCount, len(events))
	}
	received := make(chan any, len(events))
	removeListener := wailsApp.Event.On("melodex:job-progress", func(event *application.CustomEvent) {
		received <- event.Data
	})
	t.Cleanup(removeListener)

	for _, job := range events {
		app.emitJobProgressLocked(job)
	}

	logText := output.String()
	emitted := 0
	measuredPayloadBytes := 0
	for _, line := range strings.Split(logText, "\n") {
		if !strings.Contains(line, "event=job_progress_event_emitted") {
			continue
		}
		emitted++
		const marker = `payload_bytes="`
		start := strings.Index(line, marker)
		if start < 0 {
			t.Fatalf("synthetic progress event is missing payload size: %q", line)
		}
		start += len(marker)
		end := strings.IndexByte(line[start:], '"')
		if end < 0 {
			t.Fatalf("synthetic progress event has malformed payload size: %q", line)
		}
		payloadBytes, err := strconv.Atoi(line[start : start+end])
		if err != nil {
			t.Fatalf("parse synthetic progress payload size from %q: %v", line, err)
		}
		measuredPayloadBytes += payloadBytes
	}

	if emitted != expectedEventCount {
		t.Fatalf("expected %d synthetic parent/child progress emissions, got %d", expectedEventCount, emitted)
	}
	if !strings.Contains(logText, `job_id="synthetic-playlist-parent"`) || !strings.Contains(logText, `job_id="synthetic-child-01"`) {
		t.Fatalf("expected parent and child identifiers in bounded instrumentation, got %q", logText)
	}
	for _, forbidden := range []string{
		"synthetic-parent-payload-content-must-not-be-logged",
		"synthetic-child-01-download-payload-content-must-not-be-logged",
		"synthetic-download-stage",
	} {
		if strings.Contains(logText, forbidden) {
			t.Fatalf("event instrumentation logged payload content %q: %q", forbidden, logText)
		}
	}

	receivedPayloadBytes := 0
	parentEvents := 0
	childEvents := 0
	for range events {
		select {
		case data := <-received:
			progress, ok := data.(JobProgressEvent)
			if !ok {
				t.Fatalf("expected JobProgressEvent, got %T", data)
			}
			serialized, err := json.Marshal(progress)
			if err != nil {
				t.Fatalf("json.Marshal synthetic progress event: %v", err)
			}
			receivedPayloadBytes += len(serialized)
			if progress.ParentJobID == "" {
				parentEvents++
			} else if progress.ParentJobID == "synthetic-playlist-parent" {
				childEvents++
			} else {
				t.Fatalf("unexpected synthetic parent relationship: %+v", progress)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for synthetic parent/child progress event")
		}
	}
	if parentEvents != 1 || childEvents != len(events)-1 {
		t.Fatalf("expected one parent and %d child events, got %d parent and %d child events", len(events)-1, parentEvents, childEvents)
	}
	if measuredPayloadBytes != receivedPayloadBytes {
		t.Fatalf("instrumented payload bytes %d do not match emitted payload bytes %d", measuredPayloadBytes, receivedPayloadBytes)
	}
	if measuredPayloadBytes != expectedPayloadByteCount {
		t.Fatalf("expected deterministic synthetic payload total of %d bytes, got %d", expectedPayloadByteCount, measuredPayloadBytes)
	}
	if measuredPayloadBytes <= 0 {
		t.Fatal("expected synthetic progress payload bytes to be measured")
	}
}

func BenchmarkSyntheticParentChildJobProgressEvents(b *testing.B) {
	b.Setenv("MELODEX_DEBUG", "false")
	wailsApp := newEventTestApp(b)
	app := &App{wailsApp: wailsApp}
	events := syntheticParentChildJobProgressEvents()
	received := make(chan any, len(events))
	removeListener := wailsApp.Event.On("melodex:job-progress", func(event *application.CustomEvent) {
		received <- event.Data
	})
	b.Cleanup(removeListener)

	b.ReportAllocs()
	b.ResetTimer()
	totalPayloadBytes := 0
	totalEvents := 0
	for iteration := 0; iteration < b.N; iteration++ {
		for _, job := range events {
			app.emitJobProgressLocked(job)
			data := <-received
			progress, ok := data.(JobProgressEvent)
			if !ok {
				b.Fatalf("expected JobProgressEvent, got %T", data)
			}
			serialized, err := json.Marshal(progress)
			if err != nil {
				b.Fatalf("json.Marshal synthetic progress event: %v", err)
			}
			totalPayloadBytes += len(serialized)
			totalEvents++
		}
	}
	b.StopTimer()

	expectedEvents := b.N * len(events)
	if totalEvents != expectedEvents {
		b.Fatalf("expected %d synthetic progress events, got %d", expectedEvents, totalEvents)
	}
	b.ReportMetric(float64(len(events)), "events/op")
	b.ReportMetric(float64(totalPayloadBytes)/float64(b.N), "payload-bytes/op")
	b.ReportMetric(float64(totalPayloadBytes)/float64(totalEvents), "payload-bytes/event")
}
