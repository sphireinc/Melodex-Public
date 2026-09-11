package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"melodex/internal/songstore"
)

func TestImportStageManifestPersistsAttemptsAndValidatesArtifacts(t *testing.T) {
	stageDir := t.TempDir()
	artifact := filepath.Join(stageDir, "audio.mp3")
	if err := os.WriteFile(artifact, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	manifest := newImportStageManifest("job-1", "https://example.test/track")
	if err := checkpointImportStage(stageDir, &manifest, importStageDownload, "running", ""); err != nil {
		t.Fatalf("checkpoint running: %v", err)
	}
	if err := checkpointImportStage(stageDir, &manifest, importStageDownload, "completed", "", artifact); err != nil {
		t.Fatalf("checkpoint completed: %v", err)
	}
	reloaded, err := loadImportStageManifest(stageDir, "job-1", "")
	if err != nil {
		t.Fatalf("reload manifest: %v", err)
	}
	checkpoint := reloaded.Stages[importStageDownload]
	if checkpoint.Attempt != 1 || reloaded.LastCompleted != importStageDownload {
		t.Fatalf("unexpected checkpoint: %+v, last=%q", checkpoint, reloaded.LastCompleted)
	}
	if !importStageArtifactPathsUsable(stageDir, checkpoint) {
		t.Fatal("expected completed artifact checkpoint to be usable")
	}
	if err := os.WriteFile(artifact, nil, 0o600); err != nil {
		t.Fatalf("empty artifact: %v", err)
	}
	if importStageArtifactPathsUsable(stageDir, checkpoint) {
		t.Fatal("expected empty artifact checkpoint to be rejected")
	}
}

func TestImportStageManifestRejectsJobMismatch(t *testing.T) {
	stageDir := t.TempDir()
	manifest := newImportStageManifest("job-1", "source")
	if err := saveImportStageManifest(stageDir, manifest); err != nil {
		t.Fatalf("save manifest: %v", err)
	}
	if _, err := loadImportStageManifest(stageDir, "job-2", ""); err == nil {
		t.Fatal("expected job mismatch error")
	}
	if _, err := loadImportStageManifest(stageDir, "job-1", "other-source"); err == nil {
		t.Fatal("expected source mismatch error")
	}
}

func TestImportStageRecoveryPlanRetriesFromFirstNonReusableStage(t *testing.T) {
	stageDir := t.TempDir()
	audio := filepath.Join(stageDir, "audio.mp3")
	if err := os.WriteFile(audio, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	manifest := newImportStageManifest("job-1", "https://example.test/track")
	if err := checkpointImportStage(stageDir, &manifest, importStageDownload, "running", ""); err != nil {
		t.Fatalf("checkpoint download running: %v", err)
	}
	if err := checkpointImportStage(stageDir, &manifest, importStageDownload, "completed", "", audio); err != nil {
		t.Fatalf("checkpoint download completed: %v", err)
	}
	if err := checkpointImportStage(stageDir, &manifest, importStageMetadata, "failed", "metadata provider unavailable"); err != nil {
		t.Fatalf("checkpoint metadata failed: %v", err)
	}

	plan := importStageRecoveryPlanForJob(stageDir, manifest, "url", false)
	if plan.RetryFrom != importStageMetadata {
		t.Fatalf("retry from %q, want %q", plan.RetryFrom, importStageMetadata)
	}
	if !plan.canReuse(importStageDownload) {
		t.Fatalf("expected download stage to be reusable: %+v", plan)
	}
	if plan.canReuse(importStageMetadata) {
		t.Fatalf("failed metadata stage must not be reusable: %+v", plan)
	}
	wantInvalidated := []string{importStageMetadata, importStageLyrics, importStageArtwork, importStageFinalize}
	if got := strings.Join(plan.InvalidatedStages, ","); got != strings.Join(wantInvalidated, ",") {
		t.Fatalf("invalidated stages %q, want %q", got, strings.Join(wantInvalidated, ","))
	}
	detail := importStageRecoveryDetail(manifest, plan)
	for _, expected := range []string{
		"Recovery: retrying from metadata",
		"reusing validated download",
		"later stages are recomputed",
	} {
		if !strings.Contains(detail, expected) {
			t.Fatalf("recovery detail %q does not contain %q", detail, expected)
		}
	}
}

func TestImportStageRecoveryDetailIsEmptyForFirstAttempt(t *testing.T) {
	manifest := newImportStageManifest("job-1", "source")
	plan := importStageRecoveryPlan{RetryFrom: importStageAudio}
	if detail := importStageRecoveryDetail(manifest, plan); detail != "" {
		t.Fatalf("first attempt should not be labeled as recovery: %q", detail)
	}
}

func TestImportStageRecoveryDetailExplainsCompleteManifestLimitation(t *testing.T) {
	manifest := newImportStageManifest("job-1", "source")
	manifest.LastCompleted = importStageFinalize
	plan := importStageRecoveryPlan{ReusableStages: []string{importStageDownload, importStageFinalize}}
	detail := importStageRecoveryDetail(manifest, plan)
	if !strings.Contains(detail, "only validated staged audio/download artifacts are reused") || !strings.Contains(detail, "later stages are recomputed") {
		t.Fatalf("complete recovery detail does not explain current behavior: %q", detail)
	}
}

func TestImportStageRecoveryPlanRejectsMissingEarlierArtifact(t *testing.T) {
	stageDir := t.TempDir()
	audio := filepath.Join(stageDir, "audio.mp3")
	if err := os.WriteFile(audio, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	manifest := newImportStageManifest("job-1", "https://example.test/track")
	if err := checkpointImportStage(stageDir, &manifest, importStageDownload, "completed", "", audio); err != nil {
		t.Fatalf("checkpoint download: %v", err)
	}
	if err := os.Remove(audio); err != nil {
		t.Fatalf("remove audio: %v", err)
	}

	plan := importStageRecoveryPlanForJob(stageDir, manifest, "url", false)
	if plan.RetryFrom != importStageDownload {
		t.Fatalf("retry from %q, want %q", plan.RetryFrom, importStageDownload)
	}
	if len(plan.ReusableStages) != 0 {
		t.Fatalf("missing artifact must not be reusable: %+v", plan)
	}
}

func TestImportStageRecoveryPlanReusesCompleteManifestAfterCleanupCrash(t *testing.T) {
	stageDir := t.TempDir()
	audio := filepath.Join(stageDir, "audio.mp3")
	if err := os.WriteFile(audio, []byte("audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	manifest := newImportStageManifest("job-1", "https://example.test/track")
	for _, stage := range []string{importStageDownload, importStageMetadata, importStageLyrics, importStageArtwork} {
		artifacts := []string(nil)
		if stage == importStageDownload {
			artifacts = []string{audio}
		}
		if err := checkpointImportStage(stageDir, &manifest, stage, "completed", "", artifacts...); err != nil {
			t.Fatalf("checkpoint %s: %v", stage, err)
		}
	}
	if err := checkpointImportStage(stageDir, &manifest, importStageFinalize, "completed", "", audio); err != nil {
		t.Fatalf("checkpoint finalize: %v", err)
	}

	plan := importStageRecoveryPlanForJob(stageDir, manifest, "url", false)
	if plan.RetryFrom != "" || len(plan.InvalidatedStages) != 0 {
		t.Fatalf("complete manifest should not retry: %+v", plan)
	}
	for _, stage := range []string{importStageDownload, importStageMetadata, importStageLyrics, importStageArtwork, importStageFinalize} {
		if !plan.canReuse(stage) {
			t.Fatalf("complete manifest did not preserve reusable stage %q: %+v", stage, plan)
		}
	}
}

func TestPreparedLocalAudioFromStageManifestRestoresOriginal(t *testing.T) {
	stageDir := t.TempDir()
	audio := filepath.Join(stageDir, "audio.mp3")
	original := filepath.Join(stageDir, "audio.source.flac")
	writeTestMP3(t, audio)
	if err := os.WriteFile(original, []byte("flac source"), 0o600); err != nil {
		t.Fatalf("write original: %v", err)
	}
	manifest := newImportStageManifest("job-1", "local-file")
	if err := checkpointImportStage(stageDir, &manifest, importStageAudio, "completed", "", audio, original); err != nil {
		t.Fatalf("checkpoint audio: %v", err)
	}

	prepared, ok := preparedLocalAudioFromStageManifest(stageDir, manifest)
	if !ok {
		t.Fatal("expected staged audio to be reusable")
	}
	if prepared.Path != audio || prepared.OriginalStagePath != original || prepared.SourceFormat != "flac" || prepared.Format != "mp3" {
		t.Fatalf("unexpected prepared audio: %+v", prepared)
	}
}

func TestImportStageFinalizationReplayRoundTripsAndValidatesRoots(t *testing.T) {
	stageDir := t.TempDir()
	libraryRoot := t.TempDir()
	audio := filepath.Join(stageDir, "audio.mp3")
	if err := os.WriteFile(audio, []byte("mp3 audio"), 0o600); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	plan, err := songstore.BuildSongStoragePlan(libraryRoot, songstore.SongMetadata{
		Title:  "Song",
		Artist: "Artist",
		Album:  "Album",
		Audio:  songstore.SongAudio{Format: "mp3"},
	})
	if err != nil {
		t.Fatalf("build storage plan: %v", err)
	}
	manifest := newImportStageManifest("job-1", "source")
	replay := importStageFinalizationReplay{
		Plan:            plan,
		AudioSourcePath: audio,
		Lyrics:          songstore.LyricsResult{Text: "lyrics", IsComplete: true},
		Metadata: songstore.SongMetadata{
			Title:  "Song",
			Artist: "Artist",
			Album:  "Album",
			Audio:  songstore.SongAudio{Format: "mp3"},
		},
		SourceKind:       "local-file",
		SourceRef:        "/music/source.mp3",
		CompletionDetail: "Recovered import",
	}
	if err := saveImportStageFinalizationReplay(stageDir, &manifest, replay); err != nil {
		t.Fatalf("save finalization replay: %v", err)
	}
	reloaded, err := loadImportStageManifest(stageDir, "job-1", "")
	if err != nil {
		t.Fatalf("reload manifest: %v", err)
	}
	got, ok := importStageFinalizationReplayForJob(stageDir, libraryRoot, reloaded)
	if !ok {
		t.Fatal("expected valid finalization replay")
	}
	if got.AudioSourcePath != replay.AudioSourcePath || got.Plan.MetadataPath != replay.Plan.MetadataPath || got.Metadata.Title != replay.Metadata.Title {
		t.Fatalf("replay changed across persistence: got=%+v want=%+v", got, replay)
	}

	outside := replay
	outside.Plan.MetadataPath = filepath.Join(t.TempDir(), "outside.metadata.json")
	if err := validateImportStageFinalizationReplay(stageDir, libraryRoot, outside); err == nil {
		t.Fatal("expected replay target outside the library root to be rejected")
	}
}

func TestImportFinalizationReplayIsIdempotent(t *testing.T) {
	libraryRoot := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(libraryRoot)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	stageDir := filepath.Join(info.IncomingDir, "job-1")
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		t.Fatalf("mkdir stage: %v", err)
	}
	audio := filepath.Join(stageDir, "audio.mp3")
	writeTestMP3(t, audio)
	metadata := songstore.SongMetadata{
		Title:  "Song",
		Artist: "Artist",
		Album:  "Album",
		Audio:  songstore.SongAudio{Format: "mp3"},
	}
	plan, err := songstore.BuildSongStoragePlan(libraryRoot, metadata)
	if err != nil {
		t.Fatalf("build storage plan: %v", err)
	}
	manifest := newImportStageManifest("job-1", "source")
	replay := importStageFinalizationReplay{
		Plan:             plan,
		AudioSourcePath:  audio,
		Lyrics:           songstore.LyricsResult{Text: "lyrics", IsComplete: true},
		Metadata:         metadata,
		SourceKind:       "local-file",
		SourceRef:        "/music/source.mp3",
		CompletionDetail: "Recovered import",
	}
	app := &App{
		store:        store,
		info:         info,
		settings:     defaultStoredSettings(libraryRoot),
		player:       newPlaybackEngine(),
		catalog:      catalogFile{Version: catalogSchemaVersion, Tracks: []TrackRecord{}},
		jobs:         []Job{{ID: "job-1", Status: "stopped"}},
		jobCancels:   map[string]context.CancelFunc{},
		workerStates: map[int]*workerState{},
	}

	if err := app.finalizeImportFromReplay("job-1", stageDir, &manifest, replay); err != nil {
		t.Fatalf("first finalization: %v", err)
	}
	if err := app.finalizeImportFromReplay("job-1", stageDir, &manifest, replay); err != nil {
		t.Fatalf("replayed finalization: %v", err)
	}
	if len(app.catalog.Tracks) != 1 {
		t.Fatalf("expected one catalog track after replay, got %d", len(app.catalog.Tracks))
	}
	if !pathExists(plan.AudioPath) || !pathExists(plan.MetadataPath) {
		t.Fatalf("expected durable audio and metadata after replay: audio=%v metadata=%v", pathExists(plan.AudioPath), pathExists(plan.MetadataPath))
	}
	if got := app.catalog.Tracks[0].MetadataPath; got != plan.MetadataPath {
		t.Fatalf("catalog metadata path = %q, want %q", got, plan.MetadataPath)
	}
}

func TestImportFinalizationRejectsMalformedStagedAudio(t *testing.T) {
	libraryRoot := filepath.Join(t.TempDir(), "Music")
	store, info, err := newStore(libraryRoot)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	stageDir := filepath.Join(info.IncomingDir, "invalid-audio")
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		t.Fatalf("mkdir stage: %v", err)
	}
	audio := filepath.Join(stageDir, "corrupt.mp3")
	if err := os.WriteFile(audio, []byte("not an MP3"), 0o600); err != nil {
		t.Fatalf("write corrupt audio: %v", err)
	}
	metadata := songstore.SongMetadata{
		Title:  "Invalid Song",
		Artist: "Invalid Artist",
		Album:  "Invalid Album",
		Audio:  songstore.SongAudio{Format: "mp3"},
	}
	plan, err := songstore.BuildSongStoragePlan(libraryRoot, metadata)
	if err != nil {
		t.Fatalf("build storage plan: %v", err)
	}
	manifest := newImportStageManifest("invalid-audio", "source")
	app := &App{
		store:        store,
		info:         info,
		settings:     defaultStoredSettings(libraryRoot),
		player:       newPlaybackEngine(),
		catalog:      catalogFile{Version: catalogSchemaVersion, Tracks: []TrackRecord{}},
		jobs:         []Job{{ID: "invalid-audio", Status: "stopped"}},
		jobCancels:   map[string]context.CancelFunc{},
		workerStates: map[int]*workerState{},
	}
	replay := importStageFinalizationReplay{
		Plan:            plan,
		AudioSourcePath: audio,
		Metadata:        metadata,
		SourceKind:      "local-file",
		SourceRef:       "source",
	}
	if err := app.finalizeImportFromReplay("invalid-audio", stageDir, &manifest, replay); err == nil {
		t.Fatal("expected malformed staged audio to be rejected")
	}
	if pathExists(plan.AudioPath) || pathExists(plan.MetadataPath) {
		t.Fatalf("malformed audio created durable output: audio=%v metadata=%v", pathExists(plan.AudioPath), pathExists(plan.MetadataPath))
	}
	if len(app.catalog.Tracks) != 0 {
		t.Fatalf("malformed audio created catalog tracks: %d", len(app.catalog.Tracks))
	}
}

// TestImportFinalizationReplayProcessBoundary crosses a real test-process
// boundary instead of only invoking the replay function twice in one process.
// The child exits at the two durable boundaries that have historically been
// vulnerable to an interrupted import: after the replay record is persisted
// but before finalization, and after catalog/checkpoint persistence but before
// Incoming cleanup. The parent then starts from disk and runs the normal local
// import recovery path.
func TestImportFinalizationReplayProcessBoundary(t *testing.T) {
	for _, mode := range []string{"before-finalization", "after-finalization"} {
		t.Run(mode, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "Music")
			_, info, err := newStore(root)
			if err != nil {
				t.Fatalf("newStore: %v", err)
			}
			jobID := "process-boundary-" + strings.ReplaceAll(mode, "-", "")
			stageDir := filepath.Join(info.IncomingDir, jobID)
			if err := os.MkdirAll(stageDir, 0o755); err != nil {
				t.Fatalf("mkdir stage: %v", err)
			}
			audio := filepath.Join(stageDir, "audio.mp3")
			writeTestMP3(t, audio)
			metadata := songstore.SongMetadata{
				Title:  "Process Boundary Song",
				Artist: "Process Boundary Artist",
				Album:  "Process Boundary Album",
				Audio:  songstore.SongAudio{Format: "mp3"},
			}
			plan, err := songstore.BuildSongStoragePlan(root, metadata)
			if err != nil {
				t.Fatalf("BuildSongStoragePlan: %v", err)
			}
			source := filepath.Join(t.TempDir(), "source.mp3")
			manifest := newImportStageManifest(jobID, source)
			replay := importStageFinalizationReplay{
				Plan:             plan,
				AudioSourcePath:  audio,
				Lyrics:           songstore.LyricsResult{Text: "process boundary lyrics", IsComplete: true},
				Metadata:         metadata,
				SourceKind:       "local-file",
				SourceRef:        source,
				CompletionDetail: "Process-boundary recovery",
			}
			if err := saveImportStageFinalizationReplay(stageDir, &manifest, replay); err != nil {
				t.Fatalf("save replay: %v", err)
			}

			cmd := exec.Command(os.Args[0], "-test.run", "^TestImportFinalizationReplayProcessChild$")
			cmd.Env = append(os.Environ(),
				"MELODEX_IMPORT_REPLAY_CHILD=1",
				"MELODEX_IMPORT_REPLAY_MODE="+mode,
				"MELODEX_IMPORT_REPLAY_ROOT="+root,
				"MELODEX_IMPORT_REPLAY_JOB="+jobID,
			)
			if err := cmd.Run(); err == nil {
				t.Fatal("child unexpectedly exited successfully; expected simulated crash")
			} else if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 97 {
				t.Fatalf("child exit = %v; want simulated crash exit 97", err)
			}

			persistedStore, persistedInfo, err := newStore(root)
			if err != nil {
				t.Fatalf("reopen store: %v", err)
			}
			catalog, err := persistedStore.loadCatalog()
			if err != nil {
				t.Fatalf("load catalog after child crash: %v", err)
			}
			app := &App{
				store:        persistedStore,
				info:         persistedInfo,
				settings:     defaultStoredSettings(root),
				player:       newPlaybackEngine(),
				catalog:      catalog,
				jobs:         []Job{{ID: jobID, Kind: "local-file", Input: source, Status: "stopped"}},
				jobCancels:   map[string]context.CancelFunc{},
				workerStates: map[int]*workerState{},
			}

			if mode == "before-finalization" {
				// The source is intentionally absent. A replay must be selected before
				// the normal source-stat check or recovery would rediscover the input.
				if _, err := app.processLocalFileJob(context.Background(), app.jobs[0]); err != nil {
					t.Fatalf("replay after pre-finalization crash: %v", err)
				}
			} else {
				// The child persisted the catalog and completed checkpoint, then exited
				// before the parent had a chance to remove Incoming/<job-id>.
				if len(catalog.Tracks) != 1 {
					t.Fatalf("child catalog tracks = %d, want 1", len(catalog.Tracks))
				}
				if _, err := app.processLocalFileJob(context.Background(), app.jobs[0]); err != nil {
					t.Fatalf("replay after post-finalization crash: %v", err)
				}
			}
			if len(app.catalog.Tracks) != 1 {
				t.Fatalf("catalog tracks after recovery = %d, want 1", len(app.catalog.Tracks))
			}
			if pathExists(stageDir) {
				t.Fatalf("recovered process-boundary stage still exists: %s", stageDir)
			}
		})
	}
}

// TestImportFinalizationReplayProcessChild is invoked by the parent test in a
// separate test process. It intentionally exits with a non-zero code after
// the selected crash boundary; this models abrupt application termination
// without relying on a panic or host-specific signal behavior.
func TestImportFinalizationReplayProcessChild(t *testing.T) {
	if os.Getenv("MELODEX_IMPORT_REPLAY_CHILD") != "1" {
		return
	}
	root := os.Getenv("MELODEX_IMPORT_REPLAY_ROOT")
	jobID := os.Getenv("MELODEX_IMPORT_REPLAY_JOB")
	mode := os.Getenv("MELODEX_IMPORT_REPLAY_MODE")
	store, info, err := newStore(root)
	if err != nil {
		t.Fatalf("child newStore: %v", err)
	}
	stageDir := filepath.Join(info.IncomingDir, jobID)
	manifest, err := loadImportStageManifest(stageDir, jobID, "")
	if err != nil {
		t.Fatalf("child load manifest: %v", err)
	}
	replay, ok := importStageFinalizationReplayForJob(stageDir, info.LibraryRoot, manifest)
	if !ok {
		t.Fatal("child could not validate durable replay")
	}
	if mode == "after-finalization" {
		catalog, err := store.loadCatalog()
		if err != nil {
			t.Fatalf("child load catalog: %v", err)
		}
		app := &App{
			store:        store,
			info:         info,
			settings:     defaultStoredSettings(root),
			player:       newPlaybackEngine(),
			catalog:      catalog,
			jobs:         []Job{{ID: jobID, Kind: "local-file", Status: "stopped"}},
			jobCancels:   map[string]context.CancelFunc{},
			workerStates: map[int]*workerState{},
		}
		if err := app.finalizeImportFromReplay(jobID, stageDir, &manifest, replay); err != nil {
			t.Fatalf("child finalization: %v", err)
		}
	}
	os.Exit(97)
}
