package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"melodex/internal/songstore"
)

type aiOutcome struct {
	Ran     bool
	Success bool
	Status  string
	Message string
}

type workerState struct {
	cancel    context.CancelFunc
	busy      bool
	workClass string
}

type workerWorkClass string

const (
	workerWorkClassAudio      workerWorkClass = "audio"
	workerWorkClassVideo      workerWorkClass = "video"
	workerWorkClassEnrichment workerWorkClass = "enrichment"
	workerWorkClassLyrics     workerWorkClass = "lyrics"
	workerWorkClassArtwork    workerWorkClass = "artwork"
)

const (
	workerBlockedReasonQueuePaused    = "queue-paused"
	workerBlockedReasonVideoCapacity  = "video-capacity"
	workerBlockedReasonWorkerCapacity = "worker-capacity"
	workerBlockedReasonThrottle       = "throttled"
	workerThrottleReasonAuthCheck     = "yt-dlp-auth-or-bot-check"
)

type downloadStats struct {
	durations []time.Duration
	throttle  time.Time
}

type boundedBuffer struct {
	max  int
	data []byte
}

func newBoundedBuffer(max int) *boundedBuffer {
	return &boundedBuffer{max: max}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.max <= 0 {
		return len(p), nil
	}
	if len(p) >= b.max {
		b.data = append(b.data[:0], p[len(p)-b.max:]...)
		return len(p), nil
	}
	if len(b.data)+len(p) > b.max {
		excess := len(b.data) + len(p) - b.max
		if excess >= len(b.data) {
			b.data = b.data[:0]
		} else {
			b.data = append(b.data[:0], b.data[excess:]...)
		}
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *boundedBuffer) Bytes() []byte {
	return append([]byte(nil), b.data...)
}

type JobProgressEvent struct {
	JobID             string           `json:"jobId"`
	DownloadProgress  int              `json:"downloadProgress"`
	MetadataProgress  int              `json:"metadataProgress"`
	LyricsProgress    int              `json:"lyricsProgress"`
	Status            string           `json:"status"`
	Detail            string           `json:"detail,omitempty"`
	StageStatuses     JobStageStatuses `json:"stageStatuses,omitempty"`
	ParentJobID       string           `json:"parentJobId,omitempty"`
	PlaylistTotal     int              `json:"playlistTotal,omitempty"`
	PlaylistProcessed int              `json:"playlistProcessed,omitempty"`
	PlaylistFailed    int              `json:"playlistFailed,omitempty"`
}

type playlistDiscovery struct {
	Title      string
	Uploader   string
	Channel    string
	WebpageURL string
	Entries    []playlistDiscoveryEntry
}

type playlistDiscoveryEntry struct {
	ID            string
	Title         string
	URL           string
	WebpageURL    string
	Uploader      string
	Channel       string
	PlaylistIndex int
}

func (a *App) runWorkerManager(ctx context.Context) {
	a.reconcileWorkerPool(ctx)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.stopAllWorkers()
			return
		case <-ticker.C:
			a.reconcileWorkerPool(ctx)
		case <-a.workerAdjust:
			a.reconcileWorkerPool(ctx)
		}
	}
}

func (a *App) runWorker(ctx context.Context, workerID int) {
	log.Printf("worker online: %d", workerID)
	defer func() {
		a.workerMu.Lock()
		delete(a.workerStates, workerID)
		a.workerMu.Unlock()
		log.Printf("worker offline: %d", workerID)
		a.requestWorkerPoolReconcile()
	}()
	for {
		jobID, ok := a.nextQueuedJob(ctx)
		if !ok {
			return
		}
		if !a.shouldProcessJob(jobID) {
			continue
		}
		a.setWorkerBusy(workerID, true, a.workerWorkClassForJob(jobID))
		a.processJob(ctx, jobID)
		a.setWorkerBusy(workerID, false)
	}
}

func (a *App) nextQueuedJob(ctx context.Context) (string, bool) {
	wait := 100 * time.Millisecond
	for {
		if ctx.Err() != nil {
			return "", false
		}
		a.mu.Lock()
		if !a.queueFrozen && len(a.jobQueue) > 0 {
			if jobID, ok := a.nextEligibleQueuedJobLocked(); ok {
				a.mu.Unlock()
				return jobID, true
			}
		}
		a.mu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return "", false
		case <-timer.C:
		}
	}
}

func (a *App) requestWorkerPoolReconcile() {
	select {
	case a.workerAdjust <- struct{}{}:
	default:
	}
}

func (a *App) stopAllWorkers() {
	a.workerMu.Lock()
	states := make([]*workerState, 0, len(a.workerStates))
	for _, state := range a.workerStates {
		states = append(states, state)
	}
	a.workerMu.Unlock()
	for _, state := range states {
		state.cancel()
	}
}

func (a *App) reconcileWorkerPool(ctx context.Context) {
	desired := a.desiredWorkerCountLocked()
	a.workerMu.Lock()
	current := len(a.workerStates)
	if desired > current {
		for i := 0; i < desired-current; i++ {
			a.workerNextID++
			workerID := a.workerNextID
			workerCtx, cancel := context.WithCancel(ctx)
			a.workerStates[workerID] = &workerState{cancel: cancel}
			go a.runWorker(workerCtx, workerID)
		}
	} else if desired < current {
		for id, state := range a.workerStates {
			if state.busy {
				continue
			}
			state.cancel()
			delete(a.workerStates, id)
			current--
			if current <= desired {
				break
			}
		}
	}
	a.workerMu.Unlock()
}

func (a *App) setWorkerBusy(workerID int, busy bool, workClass ...workerWorkClass) {
	a.workerMu.Lock()
	if state, ok := a.workerStates[workerID]; ok {
		state.busy = busy
		if len(workClass) > 0 {
			state.workClass = string(workClass[0])
		} else if !busy {
			state.workClass = ""
		}
	}
	a.workerMu.Unlock()
	a.requestWorkerPoolReconcile()
}

func (a *App) workerWorkClassForJob(jobID string) workerWorkClass {
	a.mu.Lock()
	defer a.mu.Unlock()
	job, ok := a.jobByIDLocked(jobID)
	if !ok || job.DownloadVideo {
		if ok && job.DownloadVideo {
			return workerWorkClassVideo
		}
		return workerWorkClassAudio
	}
	return workerWorkClassAudio
}

func (a *App) desiredWorkerCountLocked() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.queueFrozen {
		return 1
	}
	now := time.Now().UTC()
	if !a.downloadStats.throttle.IsZero() && now.Before(a.downloadStats.throttle) {
		return 1
	}
	queued := 0
	running := 0
	for _, job := range a.jobs {
		switch job.Status {
		case "queued":
			queued++
		case "running":
			running++
		}
	}
	backlog := queued + running
	if backlog <= 0 {
		return 1
	}
	avg := a.averageDownloadDurationLocked()
	maxWorkers := a.maxConcurrentDownloadsLocked()
	if backlog >= 100 && avg <= 10*time.Minute {
		return minInt(3, maxWorkers)
	}
	if backlog >= 20 && avg <= 12*time.Minute {
		return minInt(2, maxWorkers)
	}
	if avg > 15*time.Minute {
		return 1
	}
	if backlog >= 50 {
		return minInt(2, maxWorkers)
	}
	return minInt(1, maxWorkers)
}

func (a *App) averageDownloadDurationLocked() time.Duration {
	if len(a.downloadStats.durations) == 0 {
		return 0
	}
	var total time.Duration
	for _, duration := range a.downloadStats.durations {
		total += duration
	}
	return total / time.Duration(len(a.downloadStats.durations))
}

func (a *App) recordDownloadOutcome(duration time.Duration, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		if !a.settings.ThrottleOnYTDLPBotErrors {
			a.downloadStats.throttle = time.Time{}
			return
		}
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "sign in to confirm") ||
			strings.Contains(lower, "not a bot") ||
			strings.Contains(lower, "cookies-from-browser") ||
			strings.Contains(lower, "cookies") {
			a.downloadStats.throttle = time.Now().Add(10 * time.Minute)
			log.Printf("yt-dlp auth/bot-check encountered; throttling worker pool until %s", a.downloadStats.throttle.Format(time.RFC3339))
		}
		return
	}
	a.downloadStats.durations = append(a.downloadStats.durations, duration)
	if len(a.downloadStats.durations) > 8 {
		a.downloadStats.durations = append([]time.Duration(nil), a.downloadStats.durations[len(a.downloadStats.durations)-8:]...)
	}
	if !a.downloadStats.throttle.IsZero() && time.Now().UTC().After(a.downloadStats.throttle) {
		a.downloadStats.throttle = time.Time{}
	}
}

func (a *App) nextEligibleQueuedJobLocked() (string, bool) {
	activeVideo := a.activeVideoDownloadsCount()
	maxVideo := a.maxConcurrentVideoDownloadsLocked()
	for idx, jobID := range a.jobQueue {
		job, ok := a.jobByIDLocked(jobID)
		if !ok || job.Status != "queued" {
			continue
		}
		if job.DownloadVideo && activeVideo >= maxVideo {
			logEvent("queue_job_waiting_video_slot", "job_id", job.ID, "active_video", activeVideo, "max_video", maxVideo)
			continue
		}
		a.jobQueue = append(a.jobQueue[:idx], a.jobQueue[idx+1:]...)
		return jobID, true
	}
	return "", false
}

func (a *App) maxConcurrentDownloadsLocked() int {
	if value := a.settings.MaxConcurrentDownloads; value > 0 {
		return value
	}
	return 2
}

func (a *App) maxConcurrentVideoDownloadsLocked() int {
	if value := a.settings.MaxConcurrentVideoDownloads; value > 0 {
		return value
	}
	return 1
}

func (a *App) maxConcurrentEnrichmentRequestsLocked() int {
	if value := a.settings.MaxConcurrentEnrichmentRequests; value > 0 {
		return value
	}
	return 2
}

func (a *App) maxConcurrentLyricsRequestsLocked() int {
	if value := a.settings.MaxConcurrentLyricsRequests; value > 0 {
		return value
	}
	return 2
}

func (a *App) activeVideoDownloadsCount() int {
	a.throughputMu.Lock()
	defer a.throughputMu.Unlock()
	return a.activeVideoDownloads
}

func (a *App) activeThroughputCounts() (video, enrichment, artwork, lyrics int) {
	a.throughputMu.Lock()
	defer a.throughputMu.Unlock()
	return a.activeVideoDownloads, a.activeEnrichmentRequests, a.activeArtworkRequests, a.activeLyricsRequests
}

func (a *App) withThroughputSlot(ctx context.Context, active *int, max int) error {
	if max <= 0 {
		max = 1
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		a.throughputMu.Lock()
		if *active < max {
			*active = *active + 1
			a.throughputMu.Unlock()
			return nil
		}
		a.throughputMu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (a *App) releaseThroughputSlot(active *int) {
	a.throughputMu.Lock()
	if *active > 0 {
		*active = *active - 1
	}
	a.throughputMu.Unlock()
}

func (a *App) withVideoDownloadSlot(ctx context.Context, fn func() error) error {
	if err := a.withThroughputSlot(ctx, &a.activeVideoDownloads, a.maxConcurrentVideoDownloadsLocked()); err != nil {
		return err
	}
	defer a.releaseThroughputSlot(&a.activeVideoDownloads)
	return fn()
}

func (a *App) withEnrichmentSlot(ctx context.Context, fn func() error) error {
	if err := a.withThroughputSlot(ctx, &a.activeEnrichmentRequests, a.maxConcurrentEnrichmentRequestsLocked()); err != nil {
		return err
	}
	defer a.releaseThroughputSlot(&a.activeEnrichmentRequests)
	return fn()
}

func (a *App) withArtworkSlot(ctx context.Context, fn func() error) error {
	if err := a.withThroughputSlot(ctx, &a.activeEnrichmentRequests, a.maxConcurrentEnrichmentRequestsLocked()); err != nil {
		return err
	}
	a.throughputMu.Lock()
	a.activeArtworkRequests++
	a.throughputMu.Unlock()
	defer func() {
		a.releaseThroughputSlot(&a.activeEnrichmentRequests)
		a.throughputMu.Lock()
		if a.activeArtworkRequests > 0 {
			a.activeArtworkRequests--
		}
		a.throughputMu.Unlock()
	}()
	return fn()
}

func (a *App) withLyricsSlot(ctx context.Context, fn func() error) error {
	if err := a.withThroughputSlot(ctx, &a.activeLyricsRequests, a.maxConcurrentLyricsRequestsLocked()); err != nil {
		return err
	}
	defer a.releaseThroughputSlot(&a.activeLyricsRequests)
	return fn()
}

func (a *App) ytDLPAuthArgs(args []string) ([]string, error) {
	if cookiesPath := strings.TrimSpace(a.settings.YTDLPCookiesPath); cookiesPath != "" {
		normalized, err := validateCookieFilePath(cookiesPath)
		if err != nil {
			return nil, fmt.Errorf("invalid yt-dlp cookies file: %w", err)
		}
		args = append(args, "--cookies", normalized)
	} else if cookiesBrowser := strings.TrimSpace(a.settings.YTDLPCookiesFromBrowser); cookiesBrowser != "" {
		normalized, err := validateBrowserCookieSelector(cookiesBrowser)
		if err != nil {
			return nil, fmt.Errorf("invalid yt-dlp browser cookies selector: %w", err)
		}
		args = append(args, "--cookies-from-browser", normalized)
	}
	if ua := strings.TrimSpace(a.sessionUA); ua != "" {
		args = append(args, "--user-agent", ua)
	}
	return args, nil
}

func ytDLPBaseArgs() []string {
	return []string{
		"--no-mtime",
		"--windows-filenames",
		"--no-continue",
		"--retries", "3",
		"--fragment-retries", "3",
		"--extractor-retries", "3",
		"--concurrent-fragments", "2",
		"--retry-sleep", "1",
		"--socket-timeout", "30",
	}
}

func ytDLPBrowserVideoFormatSelector() string {
	return strings.Join([]string{
		"bv*[ext=mp4][vcodec^=avc1]+ba[ext=m4a][acodec^=mp4a]",
		"bv*[ext=mp4][vcodec^=avc1]+ba[ext=m4a]",
		"b[ext=mp4][vcodec^=avc1][acodec^=mp4a]",
		"b[ext=mp4][vcodec^=avc1]",
		"b[ext=mp4]",
		"best[ext=mp4]",
		"best",
	}, "/")
}

func ytDLPVideoArgs(outputTemplate string) []string {
	return append(ytDLPBaseArgs(),
		"--write-info-json",
		"--no-playlist",
		"--merge-output-format", "mp4",
		"--check-formats",
		"-f", ytDLPBrowserVideoFormatSelector(),
		"-o", outputTemplate,
	)
}

func runCommandCapture(ctx context.Context, cmd *exec.Cmd) ([]byte, []byte, error) {
	return runCommandCaptureWithCorrelation(ctx, cmd, "")
}

func runCommandCaptureWithCorrelation(ctx context.Context, cmd *exec.Cmd, correlationID CorrelationID) ([]byte, []byte, error) {
	if correlationID == "" {
		correlationID = newCorrelationID()
	}
	commandName := commandLabel(cmd)
	logEvent("command_started", "command", commandName, "correlation_id", correlationID)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		logEvent("command_failed", "command", commandName, "correlation_id", correlationID, "error", err.Error())
		return nil, nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		logEvent("command_failed", "command", commandName, "correlation_id", correlationID, "error", err.Error())
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		logEvent("command_failed", "command", commandName, "correlation_id", correlationID, "error", err.Error())
		return nil, nil, err
	}

	stdoutBuf := newBoundedBuffer(maxCommandOutputBytes)
	stderrBuf := newBoundedBuffer(maxCommandOutputBytes)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(stdoutBuf, stdoutPipe)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(stderrBuf, stderrPipe)
	}()

	waitErr := cmd.Wait()
	wg.Wait()
	stdoutBytes := stdoutBuf.Bytes()
	stderrBytes := stderrBuf.Bytes()
	if waitErr != nil {
		logEvent("command_failed", "command", commandName, "correlation_id", correlationID, "error", waitErr.Error(), "stdout_bytes", len(stdoutBytes), "stderr_bytes", len(stderrBytes))
		recordCommandFailureWithCorrelation(cmd, "", correlationID, waitErr, stdoutBytes, stderrBytes)
		return stdoutBytes, stderrBytes, waitErr
	}
	logEvent("command_completed", "command", commandName, "correlation_id", correlationID, "stdout_bytes", len(stdoutBytes), "stderr_bytes", len(stderrBytes))
	return stdoutBytes, stderrBytes, nil
}

func (a *App) discoverPlaylist(ctx context.Context, ytdlpPath, url string) (playlistDiscovery, bool, error) {
	normalizedURL, err := validateHTTPURL(url)
	if err != nil {
		return playlistDiscovery{}, false, err
	}
	args := append(ytDLPBaseArgs(), "--skip-download", "--flat-playlist", "--dump-single-json", "--newline")
	args, err = a.ytDLPAuthArgs(args)
	if err != nil {
		return playlistDiscovery{}, false, err
	}
	args = append(args, normalizedURL)
	cmd := exec.CommandContext(ctx, ytdlpPath, args...)
	stdout, stderr, err := runCommandCapture(ctx, cmd)
	if err != nil {
		return playlistDiscovery{}, false, ytDLPErrorWithHint(err, append(stdout, stderr...))
	}
	var info struct {
		Type       string `json:"_type"`
		Title      string `json:"title"`
		Uploader   string `json:"uploader"`
		Channel    string `json:"channel"`
		WebpageURL string `json:"webpage_url"`
		Entries    []struct {
			ID            string `json:"id"`
			Title         string `json:"title"`
			URL           string `json:"url"`
			WebpageURL    string `json:"webpage_url"`
			Uploader      string `json:"uploader"`
			Channel       string `json:"channel"`
			PlaylistIndex int    `json:"playlist_index"`
		} `json:"entries"`
	}
	if err := decodeFirstJSONObject(stdout, &info); err != nil {
		logEvent(
			"playlist_discovery_parse_failed",
			"input", url,
			"error", err.Error(),
			"stdout_preview", commandOutputPreview(stdout),
			"stderr_preview", commandOutputPreview(stderr),
		)
		return playlistDiscovery{Title: filepath.Base(url), WebpageURL: normalizedURL}, false, nil
	}
	discovery := playlistDiscovery{
		Title:      firstNonEmpty(info.Title, filepath.Base(url)),
		Uploader:   info.Uploader,
		Channel:    info.Channel,
		WebpageURL: info.WebpageURL,
		Entries:    make([]playlistDiscoveryEntry, 0, len(info.Entries)),
	}
	for position, entry := range info.Entries {
		entryURL := firstNonEmpty(entry.WebpageURL, entry.URL)
		if entryURL == "" && entry.ID != "" {
			entryURL = "https://www.youtube.com/watch?v=" + entry.ID
		}
		playlistIndex := entry.PlaylistIndex
		if playlistIndex <= 0 {
			playlistIndex = position + 1
		}
		discovery.Entries = append(discovery.Entries, playlistDiscoveryEntry{
			ID:            entry.ID,
			Title:         entry.Title,
			URL:           entryURL,
			WebpageURL:    entry.WebpageURL,
			Uploader:      entry.Uploader,
			Channel:       entry.Channel,
			PlaylistIndex: playlistIndex,
		})
	}
	return discovery, strings.EqualFold(info.Type, "playlist") || info.Entries != nil, nil
}

func (a *App) processJob(ctx context.Context, jobID string) {
	jobCtx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.jobCancels[jobID] = cancel
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.jobCancels, jobID)
		a.mu.Unlock()
		cancel()
	}()

	job := a.updateJob(jobID, func(j *Job) {
		j.Status = "running"
		j.StartedAt = time.Now().UTC()
		switch {
		case j.Kind == "playlist" && j.ParentJobID == "":
			j.Detail = "This is a playlist. Processing time extended."
		case j.ParentJobID != "":
			j.Detail = fmt.Sprintf(
				"Playlist item %d/%d: %s",
				playlistItemIndexOrFallback(j.PlaylistIndex, j.CreatedAt),
				maxInt(j.PlaylistTotalItems, 1),
				firstNonEmpty(j.PlaylistItemTitle, j.Input),
			)
		default:
			j.Detail = "Processing import"
		}
	})
	log.Printf("job started: %s (%s)", job.ID, job.Kind)
	logEvent("job_started", "job_id", job.ID, "kind", job.Kind, "input", job.Input, "parent_job_id", job.ParentJobID)

	completionDetail := "Track cataloged"
	switch job.Kind {
	case "local-file":
		detail, err := a.processLocalFileJob(jobCtx, job)
		if detail != "" {
			completionDetail = detail
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(jobCtx.Err(), context.Canceled) {
				a.finishJob(job.ID, "stopped", "Stopped by user")
				return
			}
			a.failJob(job.ID, err)
			return
		}
	case "url":
		urlCtx, cancelURL := context.WithTimeout(jobCtx, maxExternalImportDuration)
		detail, playlistStarted, err := a.processURLJob(urlCtx, job)
		cancelURL()
		if detail != "" {
			completionDetail = detail
		}
		if playlistStarted {
			return
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(jobCtx.Err(), context.Canceled) {
				a.finishJob(job.ID, "stopped", "Stopped by user")
				return
			}
			a.failJob(job.ID, err)
			return
		}
		a.recordImportHistory(job.Input)
	case "reprocess-track":
		detail, err := a.processTrackReprocessJob(jobCtx, job)
		if detail != "" {
			completionDetail = detail
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(jobCtx.Err(), context.Canceled) {
				a.finishJob(job.ID, "stopped", "Stopped by user")
				return
			}
			a.failJob(job.ID, err)
			return
		}
	default:
		a.failJob(job.ID, fmt.Errorf("unknown job type %q", job.Kind))
		return
	}

	a.finishJob(job.ID, "completed", completionDetail)
	log.Printf("job completed: %s (%s)", job.ID, job.Kind)
	logEvent("job_completed", "job_id", job.ID, "kind", job.Kind, "input", job.Input)
}

func (a *App) processLocalFileJob(ctx context.Context, job Job) (string, error) {
	source := job.Input
	stageDir, stageManifest, err := loadJobImportStageManifest(a, job)
	if err != nil {
		return "", err
	}
	if replay, ok := importFinalizationReplayForJob(a, stageDir, *stageManifest); ok {
		if err := a.finalizeImportFromReplay(job.ID, stageDir, stageManifest, replay); err != nil {
			return "", err
		}
		a.setJobProgress(job.ID, 100, 100, 100)
		success := true
		defer func() {
			if !success {
				return
			}
			if err := os.RemoveAll(stageDir); err != nil {
				log.Printf("failed to clean incoming stage for recovered job %s: %v", job.ID, err)
			}
		}()
		return firstNonEmpty(replay.CompletionDetail, "Recovered durable finalization"), nil
	}
	if _, err := os.Stat(source); err != nil {
		return "", err
	}
	recoveryPlan := importStageRecoveryPlanForJob(stageDir, *stageManifest, job.Kind, job.DownloadVideo)
	logImportStageRecovery(job, recoveryPlan)
	a.publishImportStageRecoveryDetail(job.ID, *stageManifest, recoveryPlan)
	ffmpegPath := a.toolPath(a.settings.FFmpegPath, "ffmpeg")
	success := false
	defer func() {
		if !success {
			return
		}
		if err := os.RemoveAll(stageDir); err != nil {
			log.Printf("failed to clean incoming stage for job %s: %v", job.ID, err)
			return
		}
		log.Printf("incoming stage cleaned: %s", stageDir)
	}()
	a.setJobProgress(job.ID, 0, 0, 0)
	sourceHash, err := fileSHA256(source)
	if err != nil {
		return "", err
	}
	if existing, ok := a.localImportAlreadyExists(source, sourceHash); ok {
		log.Printf("local import duplicate skipped: %s", source)
		logEvent("import_duplicate_skipped", "kind", "local_file", "source_path", source, "existing_track_id", existing.ID)
		a.setJobResult(job.ID, existing.Title, existing.Artist, existing.Album)
		a.setJobProgress(job.ID, 100, 100, 100)
		a.finishJob(job.ID, "completed", "Already imported: "+existing.Title)
		return "", nil
	}

	folderHints := deriveLocalImportHints(source, job.SourceRoot)
	if folderHints.Structured {
		log.Printf("local import structure detected: %s / %s", folderHints.Artist, folderHints.Album)
		logEvent("local_import_structure_detected", "job_id", job.ID, "artist", folderHints.Artist, "album", folderHints.Album, "source_root", job.SourceRoot)
		a.updateJob(job.ID, func(j *Job) {
			j.Detail = folderHints.Note
		})
	}

	if err := ensureDir(stageDir); err != nil {
		return "", err
	}

	prepared, reusedAudio := preparedLocalAudioFromStageManifest(stageDir, *stageManifest)
	if !reusedAudio {
		recordJobImportStage(stageDir, stageManifest, importStageAudio, "running", "Preparing audio")
		prepared, err = a.prepareLocalAudioSource(ctx, source, stageDir, ffmpegPath)
		if err != nil {
			recordJobImportStage(stageDir, stageManifest, importStageAudio, "failed", err.Error())
			return "", err
		}
	} else {
		log.Printf("import stage reused: job=%s stage=%s path=%s", job.ID, importStageAudio, prepared.Path)
		logEvent("import_stage_reused", "job_id", job.ID, "stage", importStageAudio, "path", prepared.Path)
	}
	stagePath := prepared.Path
	log.Printf("local import staged: %s", stagePath)
	logEvent("local_import_staged", "job_id", job.ID, "stage_path", stagePath)
	if !reusedAudio {
		artifacts := []string{stagePath}
		if prepared.OriginalStagePath != "" {
			artifacts = append(artifacts, prepared.OriginalStagePath)
		}
		recordJobImportStage(stageDir, stageManifest, importStageAudio, "completed", "Audio staged", artifacts...)
	}
	a.setJobProgress(job.ID, 100, 0, 0)

	enrichmentInput := EnrichmentInput{
		FileName:                 filepath.Base(stagePath),
		SourceRef:                source,
		SourceKind:               "local-file",
		SourceTitle:              filepath.Base(stagePath),
		SourceDescriptionHint:    folderHints.NoteOrDefault("Imported from a local file chosen by the user."),
		LibraryRoot:              a.settings.LibraryRoot,
		TitleHint:                stringsTrimExt(filepath.Base(stagePath)),
		ArtistHint:               folderHints.Artist,
		AlbumHint:                folderHints.Album,
		DurationHint:             "",
		HasTranscript:            false,
		HasTimestampedTranscript: false,
		InputText:                "",
		Notes:                    folderHints.NoteOrDefault("Imported from a local file chosen by the user."),
	}
	existingAssets, err := loadLocalImportExistingAssets(source)
	if err != nil {
		return "", err
	}

	var metadataDraft songstore.MetadataResult
	var outcome aiOutcome
	var musicBrainzDetail string
	var lyricsResult songstore.LyricsResult
	var lyricsDetail string
	var lrclibMetadata *songstore.MetadataResult
	var metadata songstore.SongMetadata
	var sourceInfo songstore.SongSource
	var metadataContext songstore.SongAIContext
	metadataLoadedFromSidecar := existingAssets.HasMetadata
	recordJobImportStage(stageDir, stageManifest, importStageMetadata, "running", "Resolving metadata")
	if metadataLoadedFromSidecar {
		metadata = existingAssets.Metadata
		metadataDraft = metadataResultFromSongMetadata(metadata)
		outcome = aiOutcome{
			Ran:     false,
			Status:  "skipped",
			Message: "Loaded existing metadata from local sidecar.",
		}
		musicBrainzDetail = "Existing metadata loaded from local sidecar."
		log.Printf("local import metadata sidecar loaded for %s", filepath.Base(stagePath))
		logEvent("local_import_metadata_loaded", "job_id", job.ID, "source_name", filepath.Base(stagePath), "metadata_path", existingAssets.MetadataPath)
		a.setJobProgress(job.ID, 100, 100, 0)
	} else {
		a.setJobProgress(job.ID, 100, 25, 0)
		if err := a.withEnrichmentSlot(ctx, func() error {
			metadataDraft, outcome, musicBrainzDetail = a.enrichTrack(ctx, enrichmentInput)
			return nil
		}); err != nil {
			return "", err
		}
		log.Printf("metadata enrichment result for %s: %s", filepath.Base(stagePath), formatAIStatusMessage(outcome))
		logEvent("metadata_enrichment_result", "job_id", job.ID, "source_name", filepath.Base(stagePath), "status", formatAIStatusMessage(outcome), "musicBrainz", musicBrainzDetail)
		a.setJobProgress(job.ID, 100, 50, 0)
		metadataDraft = applyLocalImportHints(metadataDraft, folderHints)
		sourceInfo = songstore.SongSource{
			URL:          source,
			Provider:     "local",
			VideoTitle:   filepath.Base(stagePath),
			DownloadedAt: time.Now().UTC(),
		}
	}
	recordJobImportStage(stageDir, stageManifest, importStageMetadata, "completed", "Metadata resolved")

	resolvedInput := enrichmentInput.WithResolvedMetadata(metadataDraft, a.settings.LibraryRoot, filepath.Base(stagePath))
	needLyricsFetch := !existingAssets.HasTimedLyrics
	recordJobImportStage(stageDir, stageManifest, importStageLyrics, "running", "Resolving lyrics")
	if needLyricsFetch {
		if !metadataLoadedFromSidecar {
			a.setJobProgress(job.ID, 100, 50, 25)
		}
		if err := a.withLyricsSlot(ctx, func() error {
			lyricsResult, lyricsDetail, lrclibMetadata = a.cachedLyricsLookup(ctx, "https://lrclib.net", resolvedInput, metadataDraft)
			return nil
		}); err != nil {
			return "", err
		}
		log.Printf("lyrics lookup result for %s: %s", filepath.Base(stagePath), lyricsDetail)
		logEvent("lyrics_lookup_result", "job_id", job.ID, "source_name", filepath.Base(stagePath), "detail", lyricsDetail)
		if lrclibMetadata != nil && !metadataLoadedFromSidecar {
			metadataDraft = *lrclibMetadata
		}
		lyricsResult = mergeLocalLyricsResults(existingAssets.Lyrics, lyricsResult, !existingAssets.HasPlainLyrics, !existingAssets.HasTimedLyrics)
		a.setJobProgress(job.ID, 100, 100, 50)
	} else {
		lyricsResult = existingAssets.Lyrics
		if strings.TrimSpace(lyricsResult.Text) == "" && len(lyricsResult.TimedLyrics) > 0 {
			lyricsResult.Text = plainLyricsFromTimedLines(lyricsResult.TimedLyrics)
		}
		lyricsResult.Source = firstNonEmpty(lyricsResult.Source, "local-sidecar")
		lyricsResult.SourceLoc = firstNonEmpty(lyricsResult.SourceLoc, source)
		lyricsResult.Confidence = firstNonEmpty(lyricsResult.Confidence, "high")
		lyricsResult.IsComplete = strings.TrimSpace(lyricsResult.Text) != "" || len(lyricsResult.TimedLyrics) > 0
		if strings.TrimSpace(lyricsDetail) == "" {
			lyricsDetail = "Lyrics loaded from local sidecars"
		}
		a.setJobProgress(job.ID, 100, 100, 50)
	}
	recordJobImportStage(stageDir, stageManifest, importStageLyrics, "completed", "Lyrics resolved")
	if !metadataLoadedFromSidecar {
		resolvedInput = resolvedInput.WithResolvedMetadata(metadataDraft, a.settings.LibraryRoot, filepath.Base(stagePath))
		metadataContext = songAIContextFromInput(resolvedInput)
		metadata = buildSongMetadata(metadataDraft, sourceInfo, filepath.Base(stagePath), a.settings.Provider, a.settings.AIModel, outcome, metadataContext)
	}
	metadata.Audio.SourceFormat = prepared.SourceFormat
	metadata.Audio.Format = prepared.Format

	plan, err := songstore.BuildSongStoragePlan(a.settings.LibraryRoot, metadata)
	if err != nil {
		return "", err
	}
	if prepared.OriginalStagePath != "" {
		metadata.Audio.OriginalFormat = prepared.SourceFormat
		metadata.Audio.OriginalFilename = plan.BaseName + ".source." + prepared.SourceFormat
		metadata.Audio.OriginalPath = filepath.Join(plan.AlbumDir, metadata.Audio.OriginalFilename)
	} else {
		metadata.Audio.OriginalFormat = ""
		metadata.Audio.OriginalFilename = ""
		metadata.Audio.OriginalPath = ""
	}
	var artwork songstore.SongArtwork
	recordJobImportStage(stageDir, stageManifest, importStageArtwork, "running", "Resolving artwork")
	if pathExists(metadata.Artwork.Path) {
		artwork = metadata.Artwork
		if filepath.Clean(artwork.Path) != filepath.Clean(plan.ArtworkPath) && strings.TrimSpace(plan.ArtworkPath) != "" {
			if err := copyFile(artwork.Path, plan.ArtworkPath); err == nil {
				artwork.Path = plan.ArtworkPath
				artwork.Filename = filepath.Base(plan.ArtworkPath)
			}
		}
	} else {
		if err := a.withArtworkSlot(ctx, func() error {
			artwork = a.cachedArtworkLookup(ctx, metadata, plan)
			return nil
		}); err != nil {
			return "", err
		}
	}
	if artwork.Path != "" {
		metadata.Artwork = artwork
	}
	recordJobImportStage(stageDir, stageManifest, importStageArtwork, "completed", "Artwork resolved", artwork.Path)
	log.Printf("artwork result for %s: %s", metadata.Title, artworkCompletionMessage(metadata))
	logEvent("artwork_result", "job_id", job.ID, "title", metadata.Title, "detail", artworkCompletionMessage(metadata))
	a.setJobProgress(job.ID, 100, 75, 75)
	completionDetail := buildCompletedJobDetail("local-file", outcome, musicBrainzDetail, lyricsResult, lyricsDetail, metadata)
	replay := importStageFinalizationReplay{
		Plan:                    plan,
		AudioSourcePath:         stagePath,
		OriginalAudioSourcePath: prepared.OriginalStagePath,
		Lyrics:                  lyricsResult,
		Metadata:                metadata,
		SourceKind:              "local-file",
		SourceRef:               source,
		CompletionDetail:        completionDetail,
	}
	if err := saveImportStageFinalizationReplay(stageDir, stageManifest, replay); err != nil {
		return "", err
	}
	if err := a.finalizeImportFromReplay(job.ID, stageDir, stageManifest, replay); err != nil {
		return "", err
	}
	log.Printf("song files written: %s", plan.MetadataPath)
	logEvent("song_files_written", "job_id", job.ID, "metadata_path", plan.MetadataPath, "audio_path", plan.AudioPath)
	a.setJobProgress(job.ID, 100, 100, 100)
	success = true
	return completionDetail, nil
}

func (a *App) processURLJob(ctx context.Context, job Job) (string, bool, error) {
	ytdlpPath := a.toolPath(a.settings.YTDLPPath, "yt-dlp")
	ffmpegPath := a.toolPath(a.settings.FFmpegPath, "ffmpeg")
	stageDir, stageManifest, err := loadJobImportStageManifest(a, job)
	if err != nil {
		return "", false, err
	}
	if replay, ok := importFinalizationReplayForJob(a, stageDir, *stageManifest); ok {
		if err := a.finalizeImportFromReplay(job.ID, stageDir, stageManifest, replay); err != nil {
			return "", false, err
		}
		a.setJobProgress(job.ID, 100, 100, 100)
		success := true
		defer func() {
			if !success {
				return
			}
			if err := os.RemoveAll(stageDir); err != nil {
				log.Printf("failed to clean incoming stage for recovered job %s: %v", job.ID, err)
			}
		}()
		return firstNonEmpty(replay.CompletionDetail, "Recovered durable finalization"), false, nil
	}
	if ytdlpPath == "" {
		return "", false, errors.New("yt-dlp is not available")
	}

	discovery, isPlaylist, err := a.discoverPlaylist(ctx, ytdlpPath, job.Input)
	if err != nil {
		return "", false, err
	}
	if isPlaylist {
		logEvent("playlist_detected", "job_id", job.ID, "input", job.Input)
		detail, err := a.processPlaylistImportJob(job, discovery)
		if err != nil {
			return "", false, err
		}
		return detail, true, nil
	}
	recoveryPlan := importStageRecoveryPlanForJob(stageDir, *stageManifest, job.Kind, job.DownloadVideo)
	logImportStageRecovery(job, recoveryPlan)
	a.publishImportStageRecoveryDetail(job.ID, *stageManifest, recoveryPlan)
	success := false
	defer func() {
		if !success {
			return
		}
		if err := os.RemoveAll(stageDir); err != nil {
			log.Printf("failed to clean incoming stage for job %s: %v", job.ID, err)
			return
		}
		log.Printf("incoming stage cleaned: %s", stageDir)
	}()
	a.setJobProgress(job.ID, 0, 0, 0)

	if err := ensureDir(stageDir); err != nil {
		return "", false, err
	}

	var (
		audioPath           string
		downloadedVideoPath string
		infoJSON            ytDLPInfo
	)
	reusedDownload := false
	if recoveryPlan.canReuse(importStageDownload) {
		audioPath, downloadedVideoPath, reusedDownload = importStageDownloadArtifacts(stageDir, *stageManifest)
		if reusedDownload {
			if infoJSON, err = readYTDLPInfo(stageDir); err != nil {
				infoJSON = ytDLPInfo{WebpageURL: job.Input, Title: stringsTrimExt(filepath.Base(audioPath))}
			}
			log.Printf("import stage reused: job=%s stage=%s audio=%s video=%s", job.ID, importStageDownload, audioPath, downloadedVideoPath)
			logEvent("import_stage_reused", "job_id", job.ID, "stage", importStageDownload, "audio_path", audioPath, "video_path", downloadedVideoPath)
		}
	}
	if !reusedDownload {
		recordJobImportStage(stageDir, stageManifest, importStageDownload, "running", "Downloading source")
	}
	if !reusedDownload {
		if cachedDownload, ok := a.cachedDownloadForURL(job.Input, job.DownloadVideo); ok {
			infoJSON = cachedDownload.InfoJSON
			if job.DownloadVideo {
				downloadedVideoPath = firstNonEmpty(cachedDownload.VideoPath, cachedDownload.AudioPath)
				audioPath = cachedDownload.AudioPath
				if audioPath == "" && downloadedVideoPath != "" {
					preparedVideo, err := a.prepareLocalAudioSource(ctx, downloadedVideoPath, stageDir, ffmpegPath)
					if err != nil {
						return "", false, err
					}
					audioPath = preparedVideo.Path
				}
			} else {
				audioPath = firstNonEmpty(cachedDownload.AudioPath, cachedDownload.VideoPath)
				if audioPath == cachedDownload.VideoPath && cachedDownload.VideoPath != "" {
					preparedAudio, err := a.prepareLocalAudioSource(ctx, cachedDownload.VideoPath, stageDir, ffmpegPath)
					if err != nil {
						return "", false, err
					}
					audioPath = preparedAudio.Path
				}
			}
			if audioPath != "" {
				a.setJobProgress(job.ID, 100, 0, 0)
				if job.DownloadVideo {
					log.Printf("pipeline cache reused video download for %s", job.Input)
					logEvent("pipeline_cache_download_reused", "job_id", job.ID, "input", job.Input, "audio_path", audioPath, "video_path", downloadedVideoPath)
				} else {
					log.Printf("pipeline cache reused audio download for %s", job.Input)
					logEvent("pipeline_cache_download_reused", "job_id", job.ID, "input", job.Input, "audio_path", audioPath)
				}
			}
		}
		if audioPath == "" {
			if job.DownloadVideo {
				args := ytDLPVideoArgs(filepath.Join(stageDir, "%(title)s.%(ext)s"))
				if ffmpegPath != "" {
					args = append(args, "--ffmpeg-location", filepath.Dir(ffmpegPath))
				}
				args, err = a.ytDLPAuthArgs(args)
				if err != nil {
					return "", false, err
				}
				args = append(args, "--newline")
				args = append(args, job.Input)

				cmd := exec.CommandContext(ctx, ytdlpPath, args...)
				cmd.Dir = stageDir
				downloadStarted := time.Now()
				output, err := runCommandWithProgress(ctx, cmd, job.ID, func(progress int) {
					a.setJobProgress(job.ID, progress, 0, 0)
				})
				if err != nil {
					a.recordDownloadOutcome(time.Since(downloadStarted), err)
					logEvent("yt_dlp_download_failed", "job_id", job.ID, "input", job.Input, "error", err.Error())
					return "", false, ytDLPErrorWithHint(err, output)
				}
				a.recordDownloadOutcome(time.Since(downloadStarted), nil)
				downloadedVideoPath, err = newestVideoFile(stageDir)
				if err != nil {
					return "", false, err
				}
				preparedVideo, err := a.prepareLocalAudioSource(ctx, downloadedVideoPath, stageDir, ffmpegPath)
				if err != nil {
					return "", false, err
				}
				audioPath = preparedVideo.Path
				a.setJobProgress(job.ID, 100, 0, 0)
				log.Printf("yt-dlp video finished for %s", job.Input)
				logEvent("yt_dlp_video_download_completed", "job_id", job.ID, "input", job.Input, "video_path", downloadedVideoPath, "audio_path", audioPath)
				log.Printf("video result for %s: downloaded %s", job.Input, downloadedVideoPath)
				logEvent("video_download_completed", "job_id", job.ID, "source_url", job.Input, "video_path", downloadedVideoPath, "audio_path", audioPath)
				if infoJSON, err = readYTDLPInfo(stageDir); err != nil {
					infoJSON = ytDLPInfo{WebpageURL: job.Input, Title: stringsTrimExt(filepath.Base(audioPath))}
				}
			} else {
				args := append(ytDLPBaseArgs(), "--write-info-json", "--no-playlist", "-x", "--audio-format", "mp3", "-o", filepath.Join(stageDir, "%(title)s.%(ext)s"))
				if ffmpegPath != "" {
					args = append(args, "--ffmpeg-location", filepath.Dir(ffmpegPath))
				}
				args, err = a.ytDLPAuthArgs(args)
				if err != nil {
					return "", false, err
				}
				args = append(args, "--newline")
				args = append(args, job.Input)

				cmd := exec.CommandContext(ctx, ytdlpPath, args...)
				cmd.Dir = stageDir
				downloadStarted := time.Now()
				output, err := runCommandWithProgress(ctx, cmd, job.ID, func(progress int) {
					a.setJobProgress(job.ID, progress, 0, 0)
				})
				if err != nil {
					a.recordDownloadOutcome(time.Since(downloadStarted), err)
					logEvent("yt_dlp_download_failed", "job_id", job.ID, "input", job.Input, "error", err.Error())
					return "", false, ytDLPErrorWithHint(err, output)
				}
				a.recordDownloadOutcome(time.Since(downloadStarted), nil)
				a.setJobProgress(job.ID, 100, 0, 0)
				log.Printf("yt-dlp finished for %s", job.Input)
				logEvent("yt_dlp_download_completed", "job_id", job.ID, "input", job.Input)

				audioPath, err = newestAudioFile(stageDir)
				if err != nil {
					return "", false, err
				}
				if infoJSON, err = readYTDLPInfo(stageDir); err != nil {
					infoJSON = ytDLPInfo{WebpageURL: job.Input, Title: stringsTrimExt(filepath.Base(audioPath))}
				}
			}
		}
		recordJobImportStage(stageDir, stageManifest, importStageDownload, "completed", "Source downloaded", audioPath, downloadedVideoPath)
	}

	enrichmentInput := EnrichmentInput{
		FileName:                 filepath.Base(audioPath),
		SourceRef:                job.Input,
		SourceURL:                job.Input,
		SourceKind:               "url",
		SourceTitle:              firstNonEmpty(infoJSON.Title, filepath.Base(audioPath)),
		SourceUploader:           infoJSON.Uploader,
		SourceChannel:            infoJSON.Channel,
		SourceDescriptionHint:    infoJSON.WebpageURL,
		LibraryRoot:              a.settings.LibraryRoot,
		DurationSeconds:          infoJSON.DurationSeconds,
		DurationHint:             durationHint(infoJSON.DurationSeconds),
		HasTranscript:            false,
		HasTimestampedTranscript: false,
		InputText:                "",
		TitleHint:                firstNonEmpty(infoJSON.Title, stringsTrimExt(filepath.Base(audioPath))),
		Notes:                    "Downloaded externally via yt-dlp and ffmpeg, then imported into Melodex.",
	}
	var metadataDraft songstore.MetadataResult
	var outcome aiOutcome
	var musicBrainzDetail string
	recordJobImportStage(stageDir, stageManifest, importStageMetadata, "running", "Resolving metadata")
	if err := a.withEnrichmentSlot(ctx, func() error {
		metadataDraft, outcome, musicBrainzDetail = a.enrichTrack(ctx, enrichmentInput)
		return nil
	}); err != nil {
		return "", false, err
	}
	logEvent("metadata_pipeline_started", "job_id", job.ID, "source_url", job.Input, "source_title", enrichmentInput.SourceTitle)
	recordJobImportStage(stageDir, stageManifest, importStageMetadata, "completed", "Metadata resolved")

	sourceInfo := songstore.SongSource{
		URL:          firstNonEmpty(infoJSON.WebpageURL, job.Input),
		Provider:     firstNonEmpty(infoJSON.ExtractorKey, "youtube"),
		VideoID:      infoJSON.ID,
		VideoTitle:   firstNonEmpty(infoJSON.Title, infoJSON.FullTitle, filepath.Base(audioPath)),
		Channel:      firstNonEmpty(infoJSON.Channel, infoJSON.Uploader),
		DownloadedAt: time.Now().UTC(),
	}

	resolvedInput := enrichmentInput.WithResolvedMetadata(metadataDraft, a.settings.LibraryRoot, filepath.Base(audioPath))
	a.setJobProgress(job.ID, 100, 25, 0)
	var lyricsResult songstore.LyricsResult
	var lyricsDetail string
	var lrclibMetadata *songstore.MetadataResult
	recordJobImportStage(stageDir, stageManifest, importStageLyrics, "running", "Resolving lyrics")
	if err := a.withLyricsSlot(ctx, func() error {
		lyricsResult, lyricsDetail, lrclibMetadata = a.cachedLyricsLookup(ctx, "https://lrclib.net", resolvedInput, metadataDraft)
		return nil
	}); err != nil {
		return "", false, err
	}
	log.Printf("lyrics lookup result for %s: %s", filepath.Base(audioPath), lyricsDetail)
	logEvent("lyrics_lookup_result", "job_id", job.ID, "source_name", filepath.Base(audioPath), "detail", lyricsDetail)
	if lrclibMetadata != nil {
		metadataDraft = *lrclibMetadata
	}
	a.setJobProgress(job.ID, 100, 50, 50)
	recordJobImportStage(stageDir, stageManifest, importStageLyrics, "completed", "Lyrics resolved")
	resolvedInput = resolvedInput.WithResolvedMetadata(metadataDraft, a.settings.LibraryRoot, filepath.Base(audioPath))
	metadataContext := songAIContextFromInput(resolvedInput)
	metadata := buildSongMetadata(metadataDraft, sourceInfo, filepath.Base(audioPath), a.settings.Provider, a.settings.AIModel, outcome, metadataContext)
	if metadata.DurationSeconds == nil && infoJSON.DurationSeconds != nil {
		metadata.DurationSeconds = infoJSON.DurationSeconds
	}
	if job.DownloadVideo && downloadedVideoPath != "" {
		metadata.Video = songstore.SongVideo{
			Filename:  filepath.Base(downloadedVideoPath),
			Path:      downloadedVideoPath,
			Source:    "yt-dlp",
			URL:       sourceInfo.URL,
			FetchedAt: time.Now().UTC(),
		}
	}

	plan, err := songstore.BuildSongStoragePlan(a.settings.LibraryRoot, metadata)
	if err != nil {
		return "", false, err
	}
	if job.DownloadVideo && downloadedVideoPath != "" {
		recordJobImportStage(stageDir, stageManifest, importStageVideo, "running", "Writing video")
		finalVideoPath := plan.VideoPath
		if ext := filepath.Ext(downloadedVideoPath); ext != "" {
			base := strings.TrimSuffix(filepath.Base(plan.VideoPath), filepath.Ext(plan.VideoPath))
			finalVideoPath = filepath.Join(filepath.Dir(plan.VideoPath), base+ext)
			plan.VideoPath = finalVideoPath
		}
		if err := copyFile(downloadedVideoPath, plan.VideoPath); err != nil {
			recordJobImportStage(stageDir, stageManifest, importStageVideo, "failed", err.Error())
			logEvent("video_copy_failed", "job_id", job.ID, "source_path", downloadedVideoPath, "target_path", plan.VideoPath, "error", err.Error())
			return "", false, err
		}
		metadata.Video = songstore.SongVideo{
			Filename:  filepath.Base(plan.VideoPath),
			Path:      plan.VideoPath,
			Source:    "yt-dlp",
			URL:       sourceInfo.URL,
			FetchedAt: time.Now().UTC(),
		}
		recordJobImportStage(stageDir, stageManifest, importStageVideo, "completed", "Video durable", plan.VideoPath)
	}
	var artwork songstore.SongArtwork
	recordJobImportStage(stageDir, stageManifest, importStageArtwork, "running", "Resolving artwork")
	if err := a.withArtworkSlot(ctx, func() error {
		artwork = a.cachedArtworkLookup(ctx, metadata, plan)
		return nil
	}); err != nil {
		return "", false, err
	}
	if artwork.Path != "" {
		metadata.Artwork = artwork
	}
	recordJobImportStage(stageDir, stageManifest, importStageArtwork, "completed", "Artwork resolved", artwork.Path)
	log.Printf("artwork result for %s: %s", metadata.Title, artworkCompletionMessage(metadata))
	logEvent("artwork_result", "job_id", job.ID, "title", metadata.Title, "detail", artworkCompletionMessage(metadata))
	a.setJobProgress(job.ID, 100, 75, 75)
	completionDetail := buildCompletedJobDetail("url", outcome, musicBrainzDetail, lyricsResult, lyricsDetail, metadata)
	replay := importStageFinalizationReplay{
		Plan:             plan,
		AudioSourcePath:  audioPath,
		Lyrics:           lyricsResult,
		Metadata:         metadata,
		SourceKind:       "url",
		SourceRef:        job.Input,
		CompletionDetail: completionDetail,
	}
	if err := saveImportStageFinalizationReplay(stageDir, stageManifest, replay); err != nil {
		return "", false, err
	}
	if err := a.finalizeImportFromReplay(job.ID, stageDir, stageManifest, replay); err != nil {
		return "", false, err
	}
	a.rememberDownloadForURL(job.Input, infoJSON, plan.AudioPath, plan.VideoPath)
	log.Printf("song files written: %s", plan.MetadataPath)
	logEvent("song_files_written", "job_id", job.ID, "metadata_path", plan.MetadataPath, "audio_path", plan.AudioPath)
	a.setJobProgress(job.ID, 100, 100, 100)
	success = true
	return completionDetail, false, nil
}

func logImportStageRecovery(job Job, plan importStageRecoveryPlan) {
	if plan.RetryFrom == "" {
		return
	}
	logEvent("import_stage_recovery_planned", "job_id", job.ID, "kind", job.Kind, "retry_from", plan.RetryFrom, "reusable_stages", strings.Join(plan.ReusableStages, ","), "invalidated_stages", strings.Join(plan.InvalidatedStages, ","))
}

func (a *App) publishImportStageRecoveryDetail(jobID string, manifest importStageManifest, plan importStageRecoveryPlan) {
	detail := importStageRecoveryDetail(manifest, plan)
	if detail == "" {
		return
	}
	a.updateJob(jobID, func(job *Job) {
		job.Detail = detail
	})
}

func preparedLocalAudioFromStageManifest(stageDir string, manifest importStageManifest) (preparedLocalAudio, bool) {
	checkpoint, ok := manifest.Stages[importStageAudio]
	if !ok || !importStageArtifactPathsUsable(stageDir, checkpoint) || len(checkpoint.Artifacts) == 0 {
		return preparedLocalAudio{}, false
	}
	prepared := preparedLocalAudio{
		Path:         checkpoint.Artifacts[0],
		Format:       "mp3",
		SourceFormat: "mp3",
	}
	if strings.ToLower(filepath.Ext(prepared.Path)) != ".mp3" {
		return preparedLocalAudio{}, false
	}
	for _, artifact := range checkpoint.Artifacts[1:] {
		if strings.Contains(strings.ToLower(filepath.Base(artifact)), ".source.") {
			prepared.OriginalStagePath = artifact
			prepared.SourceFormat = mediaSourceFormatFromPath(artifact)
			break
		}
	}
	return prepared, true
}

func importStageDownloadArtifacts(stageDir string, manifest importStageManifest) (string, string, bool) {
	checkpoint, ok := manifest.Stages[importStageDownload]
	if !ok || !importStageArtifactPathsUsable(stageDir, checkpoint) || len(checkpoint.Artifacts) == 0 {
		return "", "", false
	}
	audioPath := checkpoint.Artifacts[0]
	if strings.TrimSpace(audioPath) == "" {
		return "", "", false
	}
	videoPath := ""
	if len(checkpoint.Artifacts) > 1 {
		videoPath = checkpoint.Artifacts[1]
	}
	return audioPath, videoPath, true
}

func (a *App) processTrackReprocessJob(ctx context.Context, job Job) (string, error) {
	extraContext := strings.TrimSpace(job.UserContext)
	if extraContext == "" {
		return "", errors.New("additional context is required")
	}

	track, metadataPath, existingMetadata, err := a.trackForReprocess(job.TrackID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(track.AudioPath) == "" {
		return "", fmt.Errorf("audio path is not available for track %s", track.ID)
	}

	sidecarAssets, err := loadLocalImportExistingAssets(track.AudioPath)
	if err != nil {
		return "", err
	}
	if sidecarAssets.HasMetadata {
		existingMetadata = sidecarAssets.Metadata
	}

	sourceInfo := existingMetadata.Source
	if strings.TrimSpace(sourceInfo.URL) == "" {
		sourceInfo.URL = firstNonEmpty(track.SourceRef, track.VideoURL)
	}
	if strings.TrimSpace(sourceInfo.Provider) == "" {
		sourceInfo.Provider = firstNonEmpty(track.SourceKind, existingMetadata.Source.Provider, "local")
	}
	if strings.TrimSpace(sourceInfo.VideoTitle) == "" {
		sourceInfo.VideoTitle = firstNonEmpty(track.SourceTitle, existingMetadata.Source.VideoTitle, track.Title)
	}
	if strings.TrimSpace(sourceInfo.Channel) == "" {
		sourceInfo.Channel = firstNonEmpty(track.SourceChannel, existingMetadata.Source.Channel)
	}
	if sourceInfo.DownloadedAt.IsZero() {
		sourceInfo.DownloadedAt = firstNonZeroTime(existingMetadata.Source.DownloadedAt, track.CreatedAt, time.Now().UTC())
	}

	input := EnrichmentInput{
		FileName:                 filepath.Base(track.AudioPath),
		SourceRef:                firstNonEmpty(track.SourceRef, sourceInfo.URL),
		SourceURL:                firstNonEmpty(sourceInfo.URL),
		SourceKind:               firstNonEmpty(track.SourceKind, sourceInfo.Provider),
		SourceTitle:              firstNonEmpty(track.SourceTitle, sourceInfo.VideoTitle, track.Title),
		SourceChannel:            firstNonEmpty(track.SourceChannel, sourceInfo.Channel),
		LibraryRoot:              a.settings.LibraryRoot,
		SourceDescriptionHint:    "User-requested reprocess with extra context.",
		DurationSeconds:          firstDuration(track.DurationSeconds, existingMetadata.DurationSeconds),
		DurationHint:             intHint(firstDuration(track.DurationSeconds, existingMetadata.DurationSeconds)),
		HasTranscript:            track.LyricsIncluded || existingMetadata.Lyrics.HasLyrics,
		HasTimestampedTranscript: track.HasTimedLyrics || existingMetadata.TimedLyrics.HasTimedLyrics,
		InputText:                extraContext,
		UserContext:              extraContext,
		ArtistHint:               track.Artist,
		AlbumHint:                track.Album,
		TitleHint:                track.Title,
		YearHint:                 parseOptionalYear(track.Year),
		GenreHint:                track.Genre,
		TrackNumberHint:          existingMetadata.TrackNumber,
		Notes:                    extraContext,
	}

	log.Printf("track reprocess started for %s: %s", track.ID, track.Title)
	logEvent("track_reprocess_started", "track_id", track.ID, "metadata_path", metadataPath, "source_ref", input.SourceRef)
	a.updateJob(job.ID, func(j *Job) {
		j.Detail = "Reprocessing track with added context"
		j.ResultTitle = track.Title
		j.ResultArtist = track.Artist
		j.ResultAlbum = track.Album
		j.DownloadProgress = 100
		j.MetadataProgress = 0
		j.LyricsProgress = 0
	})

	var metadataDraft songstore.MetadataResult
	var outcome aiOutcome
	var musicBrainzDetail string
	if err := a.withEnrichmentSlot(ctx, func() error {
		metadataDraft, outcome, musicBrainzDetail = a.enrichTrack(ctx, input)
		return nil
	}); err != nil {
		return "", err
	}
	resolvedInput := input.WithResolvedMetadata(metadataDraft, a.settings.LibraryRoot, filepath.Base(track.AudioPath))
	a.setJobProgress(job.ID, 100, 25, 0)

	var lyricsResult songstore.LyricsResult
	var lyricsDetail string
	var lrclibMetadata *songstore.MetadataResult
	if sidecarAssets.HasPlainLyrics || sidecarAssets.HasTimedLyrics {
		lyricsResult = sidecarAssets.Lyrics
		if strings.TrimSpace(lyricsResult.Text) == "" && len(lyricsResult.TimedLyrics) > 0 {
			lyricsResult.Text = plainLyricsFromTimedLines(lyricsResult.TimedLyrics)
		}
		lyricsResult.Source = firstNonEmpty(lyricsResult.Source, "local-sidecar")
		lyricsResult.SourceLoc = firstNonEmpty(lyricsResult.SourceLoc, track.AudioPath)
		lyricsResult.Confidence = firstNonEmpty(lyricsResult.Confidence, "high")
		lyricsResult.IsComplete = strings.TrimSpace(lyricsResult.Text) != "" || len(lyricsResult.TimedLyrics) > 0
		lyricsDetail = "Lyrics loaded from existing local sidecars."
		log.Printf("lyrics sidecar reused for reprocess %s", track.Title)
		logEvent("lyrics_sidecar_reused", "job_id", job.ID, "track_id", track.ID, "audio_path", track.AudioPath)
		a.setJobProgress(job.ID, 100, 50, 25)
	} else {
		if err := a.withLyricsSlot(ctx, func() error {
			lyricsResult, lyricsDetail, lrclibMetadata = a.cachedLyricsLookup(ctx, lrclibBaseURL, resolvedInput, metadataDraft)
			return nil
		}); err != nil {
			return "", err
		}
		log.Printf("lyrics lookup result for reprocess %s: %s", track.Title, lyricsDetail)
		logEvent("lyrics_lookup_result", "job_id", job.ID, "track_id", track.ID, "detail", lyricsDetail)
		if lrclibMetadata != nil {
			metadataDraft = *lrclibMetadata
		}
		a.setJobProgress(job.ID, 100, 50, 25)
	}

	resolvedInput = resolvedInput.WithResolvedMetadata(metadataDraft, a.settings.LibraryRoot, filepath.Base(track.AudioPath))
	metadataContext := songAIContextFromInput(resolvedInput)
	metadataContext.UserContext = extraContext
	metadata := buildSongMetadata(metadataDraft, sourceInfo, filepath.Base(track.AudioPath), a.settings.Provider, a.settings.AIModel, outcome, metadataContext)
	metadata.TrackID = track.ID
	if metadata.Audio.Path == "" {
		metadata.Audio.Path = track.AudioPath
	}
	if metadata.Audio.Filename == "" {
		metadata.Audio.Filename = filepath.Base(track.AudioPath)
	}
	if metadata.Video.Path == "" {
		preservedVideoPath := firstNonEmpty(existingMetadata.Video.Path, track.VideoPath)
		if pathExists(preservedVideoPath) {
			metadata.Video = songstore.SongVideo{
				Filename:  filepath.Base(preservedVideoPath),
				Path:      preservedVideoPath,
				Source:    firstNonEmpty(existingMetadata.Video.Source, track.SourceKind),
				URL:       firstNonEmpty(existingMetadata.Video.URL, track.VideoURL),
				FetchedAt: firstNonZeroTime(existingMetadata.Video.FetchedAt, track.GeneratedAt),
			}
		}
	}

	plan, err := songstore.BuildSongStoragePlanForReprocess(
		a.settings.LibraryRoot,
		metadata,
		track.AudioPath,
		track.LyricsPath,
		track.LRCPath,
		track.MetadataPath,
		track.VideoPath,
	)
	if err != nil {
		return "", err
	}

	if metadata.Video.Path != "" && pathExists(metadata.Video.Path) && filepath.Clean(metadata.Video.Path) != filepath.Clean(plan.VideoPath) {
		if err := copyFile(metadata.Video.Path, plan.VideoPath); err != nil {
			log.Printf("video carry-over failed for %s: %v", track.Title, err)
			logEvent("track_reprocess_video_copy_failed", "track_id", track.ID, "error", err.Error())
		} else {
			metadata.Video.Path = plan.VideoPath
			metadata.Video.Filename = filepath.Base(plan.VideoPath)
		}
	} else if metadata.Video.Path != "" && filepath.Clean(metadata.Video.Path) == filepath.Clean(plan.VideoPath) {
		metadata.Video.Filename = filepath.Base(plan.VideoPath)
	}

	var artwork songstore.SongArtwork
	if err := a.withArtworkSlot(ctx, func() error {
		artwork = a.cachedArtworkLookup(ctx, metadata, plan)
		return nil
	}); err != nil {
		return "", err
	}
	if artwork.Path != "" {
		metadata.Artwork = artwork
	}
	log.Printf("artwork result for reprocess %s: %s", metadata.Title, artworkCompletionMessage(metadata))
	logEvent("artwork_result", "job_id", job.ID, "track_id", track.ID, "title", metadata.Title, "detail", artworkCompletionMessage(metadata))
	a.setJobProgress(job.ID, 100, 75, 75)

	if err := songstore.WriteSongFiles(plan, track.AudioPath, "", lyricsResult, metadata); err != nil {
		return "", err
	}
	log.Printf("song files rewritten for reprocess: %s", plan.MetadataPath)
	logEvent("song_files_rewritten", "job_id", job.ID, "track_id", track.ID, "metadata_path", plan.MetadataPath, "audio_path", plan.AudioPath)
	a.setJobProgress(job.ID, 100, 100, 100)

	record, err := buildTrackRecord(plan, metadata, track.SourceKind, track.SourceRef, track.AudioPath)
	if err != nil {
		return "", err
	}
	record = a.trackWithArtworkMediaURL(record)
	record.ID = track.ID
	record.CreatedAt = track.CreatedAt
	record.SourceTitle = firstNonEmpty(metadata.Source.VideoTitle, track.SourceTitle)
	record.SourceChannel = firstNonEmpty(metadata.Source.Channel, track.SourceChannel)
	record.VideoURL = metadata.Video.URL
	oldTrack, replaced := a.replaceTrack(record)
	if replaced {
		a.cleanupReprocessedTrackFiles(oldTrack, record)
	}
	a.setJobResult(job.ID, metadata.Title, metadata.Artist, metadata.Album)
	log.Printf("track reprocess completed for %s: %s", track.ID, metadata.Title)
	logEvent("track_reprocess_completed", "track_id", track.ID, "metadata_path", plan.MetadataPath)
	return buildCompletedJobDetail("reprocess-track", outcome, musicBrainzDetail, lyricsResult, lyricsDetail, metadata), nil
}

func (a *App) processPlaylistImportJob(job Job, discovery playlistDiscovery) (string, error) {
	a.mu.Lock()
	rootIndex := -1
	for i := range a.jobs {
		if a.jobs[i].ID == job.ID {
			rootIndex = i
			break
		}
	}
	if rootIndex < 0 {
		a.mu.Unlock()
		return "", fmt.Errorf("playlist job %s not found", job.ID)
	}
	root := a.jobs[rootIndex]
	root.Kind = "playlist"
	root.Detail = "This is a playlist. Processing time extended."
	root.Error = ""
	root.StartedAt = time.Now().UTC()
	root.FinishedAt = time.Time{}
	root.PlaylistURL = firstNonEmpty(discovery.WebpageURL, job.Input)
	root.PlaylistTitle = discovery.Title
	root.PlaylistChannel = firstNonEmpty(discovery.Channel, discovery.Uploader)
	root.PlaylistTotalItems = len(discovery.Entries)
	root.PlaylistProcessedItems = 0
	root.PlaylistFailedItems = 0
	root.PlaylistCurrentIndex = 0
	root.PlaylistCurrentTitle = ""
	root.PlaylistItemTitle = discovery.Title
	a.jobs[rootIndex] = root
	a.syncCatalogJobLocked(root)

	for _, entry := range discovery.Entries {
		itemURL := firstNonEmpty(entry.WebpageURL, entry.URL)
		if itemURL == "" && entry.ID != "" {
			itemURL = "https://www.youtube.com/watch?v=" + entry.ID
		}
		child := Job{
			Kind:               "url",
			Input:              itemURL,
			ParentJobID:        job.ID,
			DownloadVideo:      root.DownloadVideo,
			PlaylistURL:        root.PlaylistURL,
			PlaylistTitle:      root.PlaylistTitle,
			PlaylistChannel:    root.PlaylistChannel,
			PlaylistTotalItems: len(discovery.Entries),
			PlaylistItemTitle:  firstNonEmpty(entry.Title, itemURL),
			PlaylistIndex:      entry.PlaylistIndex,
			Status:             "queued",
			Detail:             fmt.Sprintf("Playlist item %d/%d waiting for worker", entry.PlaylistIndex, len(discovery.Entries)),
			CreatedAt:          time.Now().UTC(),
		}
		a.addPlaylistJobLocked(child)
	}
	a.refreshPlaylistBatchLocked(job.ID)
	a.persistLocked()
	state := a.snapshotLocked()
	a.mu.Unlock()

	a.requestWorkerPoolReconcile()
	a.emitStateLocked(state)
	log.Printf("playlist discovered for %s: %d item(s)", job.Input, len(discovery.Entries))
	logEvent("playlist_discovered", "job_id", job.ID, "input", job.Input, "item_count", len(discovery.Entries))
	return fmt.Sprintf("This is a playlist. Processing time extended. %d item(s) queued.", len(discovery.Entries)), nil
}

func (a *App) enrichTrack(ctx context.Context, input EnrichmentInput) (songstore.MetadataResult, aiOutcome, string) {
	baseMetadata := fallbackMetadata(input)
	log.Printf("musicbrainz lookup started for %s", input.FileName)
	logEvent("musicbrainz_lookup_requested", "file_name", input.FileName, "source_url", input.SourceURL, "library_root", input.LibraryRoot)
	metadata, musicBrainzDetail, _ := a.cachedMusicBrainzLookup(ctx, input, baseMetadata)
	log.Printf("musicbrainz lookup finished for %s: %s", input.FileName, musicBrainzDetail)
	logEvent("musicbrainz_lookup_result", "file_name", input.FileName, "detail", musicBrainzDetail)

	metadata, outcome, cached := a.cachedAIMetadata(ctx, input, metadata)
	if cached {
		logEvent("pipeline_cache_hit", "stage", "ai_metadata", "file_name", input.FileName, "source_url", input.SourceURL)
	}
	if !outcome.Ran {
		log.Printf("AI enrichment unavailable for %s: missing API key or client", input.FileName)
		logEvent("metadata_ai_unavailable", "file_name", input.FileName, "source_url", input.SourceURL, "library_root", input.LibraryRoot)
	} else if outcome.Status == "failed" {
		log.Printf("metadata enrichment failed for %s: %s", input.FileName, outcome.Message)
		logEvent("metadata_ai_failed", "file_name", input.FileName, "error", outcome.Message)
	} else {
		log.Printf("ai metadata lookup finished for %s: %s", input.FileName, formatAIStatusMessage(outcome))
		logEvent("metadata_ai_result", "file_name", input.FileName, "status", formatAIStatusMessage(outcome))
	}
	a.setAIStatus(formatAIStatusMessage(outcome))
	return metadata, outcome, musicBrainzDetail
}

func mergeDeterministicMetadata(base, enrichment songstore.MetadataResult) songstore.MetadataResult {
	merged := base
	if strings.TrimSpace(merged.Title) == "" {
		merged.Title = enrichment.Title
	}
	if strings.TrimSpace(merged.Artist) == "" {
		merged.Artist = enrichment.Artist
	}
	if strings.TrimSpace(merged.Album) == "" || musicBrainzShouldReplaceAlbum(merged.Album) {
		if strings.TrimSpace(enrichment.Album) != "" && !musicBrainzShouldReplaceAlbum(enrichment.Album) {
			merged.Album = enrichment.Album
		}
	}
	if merged.TrackNumber == nil {
		merged.TrackNumber = enrichment.TrackNumber
	}
	if merged.Year == nil {
		merged.Year = enrichment.Year
	}
	if strings.TrimSpace(merged.Genre) == "" {
		merged.Genre = enrichment.Genre
	}
	if merged.DurationSeconds == nil {
		merged.DurationSeconds = enrichment.DurationSeconds
	}
	if strings.TrimSpace(enrichment.Confidence) != "" {
		merged.Confidence = enrichment.Confidence
	}
	if strings.TrimSpace(enrichment.MetadataConfidence) != "" {
		merged.MetadataConfidence = enrichment.MetadataConfidence
	}
	if strings.TrimSpace(enrichment.EnrichmentConfidence) != "" {
		merged.EnrichmentConfidence = enrichment.EnrichmentConfidence
	}
	if strings.TrimSpace(enrichment.ReleaseType) != "" {
		merged.ReleaseType = enrichment.ReleaseType
	}
	if strings.TrimSpace(enrichment.ISRC) != "" {
		merged.ISRC = enrichment.ISRC
	}
	if enrichment.ArtistLinks != (songstore.MetadataLinks{}) {
		merged.ArtistLinks = enrichment.ArtistLinks
	}
	if enrichment.AlbumLinks != (songstore.MetadataLinks{}) {
		merged.AlbumLinks = enrichment.AlbumLinks
	}
	if enrichment.SongLinks != (songstore.MetadataLinks{}) {
		merged.SongLinks = enrichment.SongLinks
	}
	if len(enrichment.ArtistTrivia) > 0 {
		merged.ArtistTrivia = append([]string(nil), enrichment.ArtistTrivia...)
	}
	if len(enrichment.AlbumTrivia) > 0 {
		merged.AlbumTrivia = append([]string(nil), enrichment.AlbumTrivia...)
	}
	if len(enrichment.SongTrivia) > 0 {
		merged.SongTrivia = append([]string(nil), enrichment.SongTrivia...)
	}
	if strings.TrimSpace(enrichment.SongMeaning) != "" {
		merged.SongMeaning = enrichment.SongMeaning
	}
	if len(enrichment.Tidbits) > 0 {
		merged.Tidbits = append([]string(nil), enrichment.Tidbits...)
	}
	if len(enrichment.Sources) > 0 {
		merged.Sources = append([]string(nil), enrichment.Sources...)
	}
	if strings.TrimSpace(enrichment.Notes) != "" {
		merged.Notes = enrichment.Notes
	}
	return merged
}

func buildCompletedJobDetail(jobKind string, outcome aiOutcome, musicBrainzDetail string, lyricsResult songstore.LyricsResult, lyricsDetail string, metadata songstore.SongMetadata) string {
	sourceLabel := "Source"
	if jobKind == "url" {
		sourceLabel = "Url"
	} else if jobKind == "reprocess-track" {
		sourceLabel = "Track"
	}

	summaryParts := []string{
		fmt.Sprintf("AI Metadata: %s", metadataAICompletionMessage(outcome)),
		fmt.Sprintf("MusicBrainz: %s", musicBrainzCompletionMessage(musicBrainzDetail)),
		fmt.Sprintf("Lyrics: %s", lyricsCompletionMessage(lyricsResult, lyricsDetail)),
		fmt.Sprintf("Video: %s", videoCompletionMessage(metadata)),
		fmt.Sprintf("Song: %s", sourceCompletionMessage(jobKind)),
		fmt.Sprintf("Album Art: %s", artworkCompletionMessage(metadata)),
	}

	lineTwo := metadata.Title
	if metadata.Artist != "" {
		lineTwo = fmt.Sprintf("%s by %s", metadata.Title, metadata.Artist)
	}
	if metadata.Album != "" {
		if lineTwo != "" {
			lineTwo = fmt.Sprintf("%s on %s", lineTwo, metadata.Album)
		} else {
			lineTwo = metadata.Album
		}
	}
	if lineTwo == "" {
		lineTwo = "Track cataloged"
	}

	sourceLine := sourceRefSummary(metadata.Source.URL, sourceLabel)
	if jobKind == "reprocess-track" {
		sourceLine = sourceLabel
	}
	if lineTwo != "" && lineTwo != "Track cataloged" {
		sourceLine = fmt.Sprintf("%s (%s)", sourceLine, lineTwo)
	}
	return fmt.Sprintf("%s\n%s", sourceLine, strings.Join(summaryParts, "; "))
}

func videoCompletionMessage(metadata songstore.SongMetadata) string {
	if strings.TrimSpace(metadata.Video.Path) != "" {
		return "Downloaded"
	}
	if strings.TrimSpace(metadata.Video.URL) != "" || strings.TrimSpace(metadata.Video.Source) != "" {
		return "Not found"
	}
	return "Skipped"
}

func sourceRefSummary(sourceURL, label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		label = "Source"
	}
	if strings.TrimSpace(sourceURL) == "" {
		return label
	}
	return fmt.Sprintf("%s %s", label, sourceURL)
}

func metadataAICompletionMessage(outcome aiOutcome) string {
	if !outcome.Ran {
		message := strings.TrimSpace(outcome.Message)
		if message != "" {
			return message
		}
		return "Skipped"
	}
	message := strings.TrimSpace(outcome.Message)
	if message != "" {
		return message
	}
	if strings.TrimSpace(outcome.Status) != "" {
		return capitalizeWord(outcome.Status)
	}
	return "Unknown"
}

func musicBrainzCompletionMessage(detail string) string {
	detail = strings.TrimSpace(strings.TrimPrefix(detail, "MusicBrainz:"))
	if detail == "" {
		return "not found"
	}
	return detail
}

func musicBrainzShouldEnrich(metadata songstore.MetadataResult, outcome aiOutcome) bool {
	if !outcome.Ran || !outcome.Success {
		return true
	}
	if strings.TrimSpace(metadata.Title) == "" || strings.TrimSpace(metadata.Artist) == "" {
		return true
	}
	if musicBrainzShouldReplaceAlbum(metadata.Album) {
		return true
	}
	if metadata.Year == nil {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(metadata.Confidence), "low") {
		return true
	}
	return false
}

func lyricsCompletionMessage(lyricsResult songstore.LyricsResult, lyricsDetail string) string {
	if strings.TrimSpace(lyricsDetail) != "" {
		detail := strings.TrimSpace(strings.TrimPrefix(lyricsDetail, "Lyrics LRCLIB:"))
		if detail != "" {
			return detail
		}
	}
	if lyricsResult.Source == "lrclib" {
		if lyricsResult.IsComplete || strings.TrimSpace(lyricsResult.Text) != "" || len(lyricsResult.TimedLyrics) > 0 {
			return "matched"
		}
		if strings.TrimSpace(lyricsResult.Notes) != "" {
			return strings.TrimSpace(lyricsResult.Notes)
		}
		return "not found"
	}
	if strings.TrimSpace(lyricsResult.Notes) != "" {
		return strings.TrimSpace(lyricsResult.Notes)
	}
	return "not found"
}

func sourceCompletionMessage(jobKind string) string {
	switch jobKind {
	case "url":
		return "Downloaded"
	case "local-file":
		return "Imported"
	case "reprocess-track":
		return "Reprocessed"
	default:
		return "Processed"
	}
}

func artworkCompletionMessage(metadata songstore.SongMetadata) string {
	if strings.TrimSpace(metadata.Artwork.Path) != "" {
		return "Downloaded"
	}
	return "Not found"
}

func buildMetadataAIStatus(success bool, metadataErr error, metadata songstore.MetadataResult) (string, string) {
	if metadataErr != nil {
		return "failed", metadataErr.Error()
	}
	if !success {
		return "partial", "Metadata discovery returned incomplete fields."
	}
	if confidence := strings.TrimSpace(firstNonEmpty(metadata.MetadataConfidence, metadata.Confidence)); confidence == "low" {
		return "partial", "Metadata discovered with low confidence."
	}
	return "success", "Metadata discovered."
}

func formatAIStatusMessage(outcome aiOutcome) string {
	if !outcome.Ran {
		return "Missing API key"
	}
	status := strings.TrimSpace(outcome.Status)
	if status == "" {
		status = "failed"
	}
	message := strings.TrimSpace(outcome.Message)
	if message == "" {
		return capitalizeWord(status)
	}
	return fmt.Sprintf("%s: %s", capitalizeWord(status), message)
}

func capitalizeWord(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func metadataHasMeaningfulData(metadata songstore.MetadataResult) bool {
	return strings.TrimSpace(metadata.Title) != "" ||
		strings.TrimSpace(metadata.Artist) != "" ||
		strings.TrimSpace(metadata.Album) != "" ||
		metadata.TrackNumber != nil ||
		metadata.Year != nil ||
		strings.TrimSpace(metadata.Genre) != "" ||
		metadata.DurationSeconds != nil
}

func buildSongMetadata(draft songstore.MetadataResult, source songstore.SongSource, audioName, aiProvider, aiModel string, outcome aiOutcome, aiContext songstore.SongAIContext) songstore.SongMetadata {
	metadataConfidence := strings.TrimSpace(draft.MetadataConfidence)
	if metadataConfidence == "" {
		metadataConfidence = strings.TrimSpace(draft.Confidence)
	}
	audioFormat := audioFormatFromName(audioName)
	if audioFormat == "" {
		audioFormat = "mp3"
	}
	metadata := songstore.SongMetadata{
		TrackID:              shortID(),
		Title:                cleanDisplayName(draft.Title),
		Artist:               cleanDisplayName(draft.Artist),
		Album:                cleanDisplayName(draft.Album),
		TrackNumber:          draft.TrackNumber,
		Year:                 draft.Year,
		Genre:                draft.Genre,
		DurationSeconds:      draft.DurationSeconds,
		Confidence:           strings.TrimSpace(draft.Confidence),
		MetadataConfidence:   metadataConfidence,
		EnrichmentConfidence: strings.TrimSpace(draft.EnrichmentConfidence),
		ReleaseType:          strings.TrimSpace(draft.ReleaseType),
		ISRC:                 strings.TrimSpace(draft.ISRC),
		ArtistLinks:          draft.ArtistLinks,
		AlbumLinks:           draft.AlbumLinks,
		SongLinks:            draft.SongLinks,
		ArtistTrivia:         append([]string(nil), draft.ArtistTrivia...),
		AlbumTrivia:          append([]string(nil), draft.AlbumTrivia...),
		SongTrivia:           append([]string(nil), draft.SongTrivia...),
		SongMeaning:          strings.TrimSpace(draft.SongMeaning),
		Tidbits:              append([]string(nil), draft.Tidbits...),
		Sources:              append([]string(nil), draft.Sources...),
		MetadataNotes:        draft.Notes,
		Source:               source,
		Audio: songstore.SongAudio{
			Filename:     audioName,
			Format:       audioFormat,
			SourceFormat: audioFormat,
		},
		AI: songstore.SongAI{
			Provider:    aiProvider,
			Model:       aiModel,
			Ran:         outcome.Ran,
			Status:      outcome.Status,
			Message:     outcome.Message,
			Context:     aiContext,
			GeneratedAt: time.Now().UTC(),
		},
	}
	if metadata.Title == "" {
		metadata.Title = cleanDisplayName(stringsTrimExt(audioName))
	}
	if metadata.Artist == "" {
		metadata.Artist = "Unknown Artist"
	}
	if metadata.Album == "" {
		metadata.Album = "Unknown Album"
	}
	return metadata
}

type preparedLocalAudio struct {
	Path              string
	Format            string
	SourceFormat      string
	OriginalStagePath string
}

func (a *App) prepareLocalAudioSource(ctx context.Context, source, stageDir, ffmpegPath string) (preparedLocalAudio, error) {
	sourceFormat := mediaSourceFormatFromPath(source)
	if sourceFormat == "" {
		return preparedLocalAudio{}, fmt.Errorf("unsupported audio format for %s", filepath.Base(source))
	}
	baseName := stringsTrimExt(filepath.Base(source))
	targetPath := filepath.Join(stageDir, baseName+".mp3")
	if sourceFormat == "mp3" {
		if err := copyFile(source, targetPath); err != nil {
			return preparedLocalAudio{}, err
		}
		return preparedLocalAudio{
			Path:         targetPath,
			Format:       "mp3",
			SourceFormat: "mp3",
		}, nil
	}
	if ffmpegPath == "" {
		return preparedLocalAudio{}, fmt.Errorf("unsupported audio format %s; configure ffmpeg to transcode it to MP3", sourceFormat)
	}
	args := []string{
		"-y",
		"-nostdin",
		"-i", source,
		"-vn",
		"-codec:a", "libmp3lame",
		"-q:a", "2",
		targetPath,
	}
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	stdout, stderr, err := runCommandCapture(ctx, cmd)
	output := append(stdout, stderr...)
	if err != nil {
		details := strings.TrimSpace(redactSensitiveText(string(output)))
		if details == "" {
			return preparedLocalAudio{}, fmt.Errorf("ffmpeg transcode failed: %w", err)
		}
		return preparedLocalAudio{}, fmt.Errorf("ffmpeg transcode failed: %w: %s", err, details)
	}
	log.Printf("local import transcoded: %s -> %s", source, targetPath)
	logEvent("local_import_transcoded", "source_path", source, "target_path", targetPath, "source_format", sourceFormat)
	prepared := preparedLocalAudio{
		Path:         targetPath,
		Format:       "mp3",
		SourceFormat: sourceFormat,
	}
	if a.settings.KeepOriginalAudio {
		originalStagePath := filepath.Join(stageDir, baseName+".source."+sourceFormat)
		if err := copyFile(source, originalStagePath); err != nil {
			return preparedLocalAudio{}, err
		}
		prepared.OriginalStagePath = originalStagePath
	}
	return prepared, nil
}

func songAIContextFromInput(input EnrichmentInput) songstore.SongAIContext {
	return songstore.SongAIContext{
		FileName:                 input.FileName,
		SourceRef:                input.SourceRef,
		SourceURL:                input.SourceURL,
		SourceKind:               input.SourceKind,
		SourceTitle:              input.SourceTitle,
		SourceUploader:           input.SourceUploader,
		SourceChannel:            input.SourceChannel,
		LibraryRoot:              input.LibraryRoot,
		SourceDescriptionHint:    input.SourceDescriptionHint,
		DurationSeconds:          input.DurationSeconds,
		DurationHint:             input.DurationHint,
		HasTranscript:            input.HasTranscript,
		HasTimestampedTranscript: input.HasTimestampedTranscript,
		InputText:                input.InputText,
		UserContext:              input.UserContext,
		ArtistHint:               input.ArtistHint,
		AlbumHint:                input.AlbumHint,
		TitleHint:                input.TitleHint,
		YearHint:                 input.YearHint,
		GenreHint:                input.GenreHint,
		TrackNumberHint:          input.TrackNumberHint,
		Notes:                    input.Notes,
		ResolvedTitle:            input.ResolvedTitle,
		ResolvedArtist:           input.ResolvedArtist,
		ResolvedAlbum:            input.ResolvedAlbum,
		ResolvedTrackNumber:      input.ResolvedTrackNumber,
		ResolvedYear:             input.ResolvedYear,
		ResolvedGenre:            input.ResolvedGenre,
		ResolvedConfidence:       input.ResolvedConfidence,
		ResolvedNotes:            input.ResolvedNotes,
		TargetArtistDir:          input.TargetArtistDir,
		TargetAlbumDir:           input.TargetAlbumDir,
		TargetBaseName:           input.TargetBaseName,
		TargetAudioPath:          input.TargetAudioPath,
		TargetLyricsPath:         input.TargetLyricsPath,
		TargetTimedLyricsPath:    input.TargetTimedLyricsPath,
		TargetMetadataPath:       input.TargetMetadataPath,
	}
}

type localImportHints struct {
	Artist     string
	Album      string
	Structured bool
	Note       string
}

func (h localImportHints) NoteOrDefault(fallback string) string {
	if strings.TrimSpace(h.Note) != "" {
		return h.Note
	}
	return fallback
}

func deriveLocalImportHints(sourcePath, sourceRoot string) localImportHints {
	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = filepath.Dir(sourcePath)
	}
	cleanRoot := filepath.Clean(sourceRoot)
	cleanPath := filepath.Clean(sourcePath)
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return localImportHints{}
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == "" {
		return localImportHints{}
	}
	parts := strings.Split(rel, "/")
	if len(parts) < 2 {
		return localImportHints{}
	}
	artist := cleanDisplayName(filepath.Base(cleanRoot))
	album := cleanDisplayName(parts[0])
	if artist == "" || album == "" {
		return localImportHints{}
	}
	note := fmt.Sprintf("Detected existing Artist / Album / Song structure: %s / %s", artist, album)
	return localImportHints{
		Artist:     artist,
		Album:      album,
		Structured: true,
		Note:       note,
	}
}

func applyLocalImportHints(draft songstore.MetadataResult, hints localImportHints) songstore.MetadataResult {
	if !hints.Structured {
		return draft
	}
	if strings.TrimSpace(hints.Artist) != "" {
		draft.Artist = hints.Artist
	}
	if strings.TrimSpace(hints.Album) != "" {
		draft.Album = hints.Album
	}
	if strings.TrimSpace(draft.Notes) == "" {
		draft.Notes = hints.Note
	} else if !strings.Contains(strings.ToLower(draft.Notes), strings.ToLower(hints.Note)) {
		draft.Notes = strings.TrimSpace(draft.Notes + " " + hints.Note)
	}
	return draft
}

func durationHint(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

type ytDLPInfo struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	FullTitle       string   `json:"fulltitle"`
	Channel         string   `json:"channel"`
	Uploader        string   `json:"uploader"`
	WebpageURL      string   `json:"webpage_url"`
	ExtractorKey    string   `json:"extractor_key"`
	Duration        *float64 `json:"duration"`
	DurationSeconds *int     `json:"-"`
}

func readYTDLPInfo(dir string) (ytDLPInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ytDLPInfo{}, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".info.json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return ytDLPInfo{}, err
		}
		var info ytDLPInfo
		if err := decodeFirstJSONObject(data, &info); err != nil {
			return ytDLPInfo{}, err
		}
		if info.Duration != nil {
			seconds := int(*info.Duration)
			info.DurationSeconds = &seconds
		}
		return info, nil
	}
	return ytDLPInfo{}, errors.New("yt-dlp info json not found")
}

func decodeFirstJSONObject(data []byte, target any) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return errors.New("empty JSON payload")
	}
	var firstErr error
	for offset := 0; offset < len(trimmed); {
		idx := bytes.IndexByte(trimmed[offset:], '{')
		if idx < 0 {
			break
		}
		start := offset + idx
		decoder := json.NewDecoder(bytes.NewReader(trimmed[start:]))
		if err := decoder.Decode(target); err == nil {
			return nil
		} else if firstErr == nil {
			firstErr = err
		}
		offset = start + 1
	}
	if firstErr != nil {
		return firstErr
	}
	return fmt.Errorf("no JSON object found in payload: %s", commandOutputPreview(trimmed))
}

func commandOutputPreview(data []byte) string {
	text := strings.TrimSpace(redactSensitiveText(string(data)))
	if len(text) > 240 {
		return text[:240] + "..."
	}
	return text
}

var ytDLPProgressPattern = regexp.MustCompile(`(\d{1,3}(?:\.\d+)?)%`)

const maxCommandOutputBytes = 256 * 1024
const maxExternalImportDuration = 2 * time.Hour

func runCommandWithProgress(ctx context.Context, cmd *exec.Cmd, jobID string, onProgress func(int)) ([]byte, error) {
	correlationID := newCorrelationID()
	commandName := commandLabel(cmd)
	logEvent("command_started", "command", commandName, "job_id", jobID, "correlation_id", correlationID)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		logEvent("command_failed", "command", commandName, "job_id", jobID, "error", err.Error())
		return nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		logEvent("command_failed", "command", commandName, "job_id", jobID, "error", err.Error())
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		logEvent("command_failed", "command", commandName, "job_id", jobID, "error", err.Error())
		return nil, err
	}

	stdoutBuf := newBoundedBuffer(maxCommandOutputBytes)
	stderrBuf := newBoundedBuffer(maxCommandOutputBytes)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(stdoutBuf, stdoutPipe)
	}()

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			_, _ = stderrBuf.Write(append([]byte(line), '\n'))
			if onProgress == nil {
				continue
			}
			if progress, ok := parseYTDLPProgress(line); ok {
				onProgress(progress)
			}
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
			_, _ = stderrBuf.Write(append([]byte(err.Error()), '\n'))
		}
	}()

	waitErr := cmd.Wait()
	wg.Wait()

	stdoutBytes := stdoutBuf.Bytes()
	stderrBytes := stderrBuf.Bytes()
	output := append(stdoutBytes, stderrBytes...)
	if waitErr != nil {
		logEvent("command_failed", "command", commandName, "job_id", jobID, "correlation_id", correlationID, "error", waitErr.Error(), "stdout_bytes", len(stdoutBytes), "stderr_bytes", len(stderrBytes))
		recordCommandFailureWithCorrelation(cmd, jobID, correlationID, waitErr, stdoutBytes, stderrBytes)
		return output, waitErr
	}
	logEvent("command_completed", "command", commandName, "job_id", jobID, "correlation_id", correlationID, "stdout_bytes", len(stdoutBytes), "stderr_bytes", len(stderrBytes))
	return output, nil
}

func parseYTDLPProgress(line string) (int, bool) {
	match := ytDLPProgressPattern.FindStringSubmatch(line)
	if len(match) < 2 {
		return 0, false
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, false
	}
	return clampProgress(int(math.Round(value))), true
}

func ytDLPErrorWithHint(err error, output []byte) error {
	details := strings.TrimSpace(redactSensitiveText(string(output)))
	lower := strings.ToLower(details)
	if strings.Contains(lower, "sign in to confirm you’re not a bot") ||
		strings.Contains(lower, "sign in to confirm you're not a bot") ||
		strings.Contains(lower, "use --cookies-from-browser or --cookies for the authentication") {
		return fmt.Errorf("yt-dlp requires authentication. Set a cookies file or cookies-from-browser in Settings > Downloader, then retry: %w", err)
	}
	if details == "" {
		return fmt.Errorf("yt-dlp failed: %w", err)
	}
	return fmt.Errorf("yt-dlp failed: %w: %s", err, details)
}

func (a *App) downloadVideoFile(ctx context.Context, sourceURL, stageDir string) (string, error) {
	ytdlpPath := a.toolPath(a.settings.YTDLPPath, "yt-dlp")
	ffmpegPath := a.toolPath(a.settings.FFmpegPath, "ffmpeg")
	if ytdlpPath == "" {
		return "", errors.New("yt-dlp is not available")
	}
	normalizedURL, err := validateHTTPURL(sourceURL)
	if err != nil {
		return "", err
	}
	if err := ensureDir(stageDir); err != nil {
		return "", err
	}
	args := append(ytDLPVideoArgs(filepath.Join(stageDir, "%(title)s.%(ext)s")), "--newline")
	if ffmpegPath != "" {
		args = append(args, "--ffmpeg-location", filepath.Dir(ffmpegPath))
	}
	args, err = a.ytDLPAuthArgs(args)
	if err != nil {
		return "", err
	}
	args = append(args, normalizedURL)
	cmd := exec.CommandContext(ctx, ytdlpPath, args...)
	cmd.Dir = stageDir
	output, err := runCommandWithProgress(ctx, cmd, "", nil)
	if err != nil {
		return "", ytDLPErrorWithHint(err, output)
	}
	videoPath, err := newestVideoFile(stageDir)
	if err != nil {
		return "", err
	}
	return videoPath, nil
}

func buildTrackRecord(plan songstore.SongStoragePlan, metadata songstore.SongMetadata, sourceKind, sourceRef, audioSourcePath string) (TrackRecord, error) {
	hash, err := fileSHA256(audioSourcePath)
	if err != nil {
		return TrackRecord{}, err
	}
	metadataInfo, _ := os.Stat(plan.MetadataPath)
	year := ""
	if metadata.Year != nil {
		year = strconv.Itoa(*metadata.Year)
	}
	record := TrackRecord{
		ID:                     metadata.TrackID,
		Artist:                 metadata.Artist,
		Album:                  metadata.Album,
		Title:                  metadata.Title,
		Genre:                  metadata.Genre,
		Year:                   year,
		DurationSeconds:        metadata.DurationSeconds,
		LyricsIncluded:         metadata.Lyrics.HasLyrics,
		BundlePath:             plan.MetadataPath,
		StorageDir:             plan.AlbumDir,
		AudioPath:              plan.AudioPath,
		LyricsPath:             plan.LyricsPath,
		LRCPath:                plan.LRCPath,
		ArtworkPath:            plan.ArtworkPath,
		VideoPath:              plan.VideoPath,
		MetadataPath:           plan.MetadataPath,
		MetadataConfidence:     metadata.MetadataConfidence,
		MetadataNotes:          metadata.MetadataNotes,
		LyricsConfidence:       metadata.Lyrics.Confidence,
		LyricsSource:           metadata.Lyrics.Source,
		LyricsSourceLoc:        metadata.Lyrics.SourceLoc,
		TimedLyricsConfidence:  metadata.TimedLyrics.Confidence,
		TimedLyricsGranularity: metadata.TimedLyrics.Granularity,
		HasTimedLyrics:         metadata.TimedLyrics.HasTimedLyrics,
		SourceTitle:            metadata.Source.VideoTitle,
		SourceChannel:          metadata.Source.Channel,
		AIProvider:             metadata.AI.Provider,
		AIModel:                metadata.AI.Model,
		GeneratedAt:            metadata.AI.GeneratedAt,
		SourceKind:             sourceKind,
		SourceRef:              sourceRef,
		VideoURL:               metadata.Video.URL,
		Hash:                   hash,
		MetadataSize:           fileSize(metadataInfo),
		MetadataModTime:        fileModTime(metadataInfo),
		CreatedAt:              time.Now().UTC(),
	}
	return record, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstDuration(values ...*int) *int {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func playlistItemIndexOrFallback(index int, createdAt time.Time) int {
	if index > 0 {
		return index
	}
	return 1
}

func maxInt(values ...int) int {
	max := 0
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}

func minInt(values ...int) int {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, value := range values[1:] {
		if value < min {
			min = value
		}
	}
	return min
}

func newestAudioFile(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".mp3", ".m4a", ".aac", ".flac", ".wav", ".ogg", ".webm", ".opus":
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", errors.New("no audio file was produced")
	}
	sort.Slice(files, func(i, j int) bool {
		infoI, _ := os.Stat(files[i])
		infoJ, _ := os.Stat(files[j])
		if infoI == nil || infoJ == nil {
			return files[i] > files[j]
		}
		return infoI.ModTime().After(infoJ.ModTime())
	})
	return files[0], nil
}

func newestVideoFile(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".mp4", ".mkv", ".webm", ".mov", ".m4v":
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", errors.New("no video file was produced")
	}
	sort.Slice(files, func(i, j int) bool {
		infoI, _ := os.Stat(files[i])
		infoJ, _ := os.Stat(files[j])
		if infoI == nil || infoJ == nil {
			return files[i] > files[j]
		}
		return infoI.ModTime().After(infoJ.ModTime())
	})
	return files[0], nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file: %s", src)
	}
	return writeAtomicStream(dst, in, info.Mode().Perm())
}
