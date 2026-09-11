package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"melodex/internal/songstore"
)

// Import stages are deliberately kept as stable strings. They are written to
// Incoming/<job-id> so a restart can inspect durable evidence instead of
// guessing from whichever temporary files happen to remain.
const (
	importStageManifestVersion = 1
	importStageDiscovery       = "discovery"
	importStageDownload        = "download"
	importStageAudio           = "audio"
	importStageVideo           = "video"
	importStageMetadata        = "metadata"
	importStageMusicBrainz     = "musicbrainz"
	importStageLyrics          = "lyrics"
	importStageArtwork         = "artwork"
	importStageEnrichment      = "enrichment"
	importStageFinalize        = "finalize"
)

const importStageManifestName = ".melodex-stage.json"

type importStageRecoveryPlan struct {
	RetryFrom         string
	ReusableStages    []string
	InvalidatedStages []string
}

type importStageCheckpoint struct {
	Stage       string    `json:"stage"`
	Status      string    `json:"status"`
	Attempt     int       `json:"attempt"`
	StartedAt   time.Time `json:"startedAt,omitempty"`
	CompletedAt time.Time `json:"completedAt,omitempty"`
	Error       string    `json:"error,omitempty"`
	Artifacts   []string  `json:"artifacts,omitempty"`
}

// importStageFinalizationReplay is the durable input set for the final write
// boundary. It is written before WriteSongFiles starts so a crash during the
// write, catalog update, or final checkpoint can replay the same destination
// paths and metadata without invoking external providers again.
//
// The fields are deliberately scoped to finalization rather than becoming a
// second orchestration state machine. A future user-selectable retry-from-
// stage policy can build on this record without changing the recovery safety
// contract.
type importStageFinalizationReplay struct {
	Plan                    songstore.SongStoragePlan `json:"plan"`
	AudioSourcePath         string                    `json:"audioSourcePath"`
	OriginalAudioSourcePath string                    `json:"originalAudioSourcePath,omitempty"`
	Lyrics                  songstore.LyricsResult    `json:"lyrics"`
	Metadata                songstore.SongMetadata    `json:"metadata"`
	SourceKind              string                    `json:"sourceKind"`
	SourceRef               string                    `json:"sourceRef"`
	CompletionDetail        string                    `json:"completionDetail,omitempty"`
}

type importStageManifest struct {
	Version       int                              `json:"version"`
	JobID         string                           `json:"jobId"`
	Source        string                           `json:"source,omitempty"`
	UpdatedAt     time.Time                        `json:"updatedAt"`
	LastCompleted string                           `json:"lastCompleted,omitempty"`
	Stages        map[string]importStageCheckpoint `json:"stages"`
	Finalization  *importStageFinalizationReplay   `json:"finalization,omitempty"`
}

func importStageManifestHasRecoveryEvidence(manifest importStageManifest) bool {
	if strings.TrimSpace(manifest.LastCompleted) != "" {
		return true
	}
	for _, checkpoint := range manifest.Stages {
		if checkpoint.Attempt > 0 || strings.TrimSpace(checkpoint.Status) != "" || len(checkpoint.Artifacts) > 0 {
			return true
		}
	}
	return false
}

func importStageRecoveryStageLabel(stage string) string {
	labels := map[string]string{
		importStageAudio:       "audio preparation",
		importStageDownload:    "download",
		importStageMetadata:    "metadata",
		importStageMusicBrainz: "MusicBrainz",
		importStageLyrics:      "lyrics",
		importStageArtwork:     "artwork",
		importStageVideo:       "video",
		importStageEnrichment:  "enrichment",
		importStageFinalize:    "finalization",
	}
	if label, ok := labels[strings.TrimSpace(stage)]; ok {
		return label
	}
	return strings.TrimSpace(stage)
}

// importStageRecoveryDetail is intentionally phrased around the capabilities
// that are implemented today. A manifest can describe completed stages, but
// only validated staged audio/download artifacts are replay inputs for the
// current pipeline. This keeps the existing Start/Retry action useful without
// implying that arbitrary later-stage retry is already supported.
func importStageRecoveryDetail(manifest importStageManifest, plan importStageRecoveryPlan) string {
	if !importStageManifestHasRecoveryEvidence(manifest) {
		return ""
	}
	if plan.RetryFrom != "" {
		detail := fmt.Sprintf("Recovery: retrying from %s", importStageRecoveryStageLabel(plan.RetryFrom))
		if len(plan.ReusableStages) > 0 {
			reusable := make([]string, 0, len(plan.ReusableStages))
			for _, stage := range plan.ReusableStages {
				reusable = append(reusable, importStageRecoveryStageLabel(stage))
			}
			detail += "; reusing validated " + strings.Join(reusable, ", ")
		} else {
			detail += "; no validated earlier stage is available"
		}
		return detail + "; later stages are recomputed"
	}
	return "Recovery: stage history found; only validated staged audio/download artifacts are reused and later stages are recomputed"
}

func loadJobImportStageManifest(a *App, job Job) (string, *importStageManifest, error) {
	if a == nil {
		return "", nil, errors.New("app is required")
	}
	stageDir := filepath.Join(a.info.IncomingDir, job.ID)
	if err := ensureDir(stageDir); err != nil {
		return "", nil, err
	}
	manifest, err := loadImportStageManifest(stageDir, job.ID, job.Input)
	if err != nil {
		return stageDir, nil, err
	}
	if err := saveImportStageManifest(stageDir, manifest); err != nil {
		return stageDir, nil, err
	}
	return stageDir, &manifest, nil
}

func recordJobImportStage(stageDir string, manifest *importStageManifest, stage, status, detail string, artifacts ...string) {
	if err := checkpointImportStage(stageDir, manifest, stage, status, detail, artifacts...); err != nil {
		logEvent("import_stage_checkpoint_failed", "stage", stage, "status", status, "error", err.Error())
	}
}

func newImportStageManifest(jobID, source string) importStageManifest {
	return importStageManifest{
		Version:   importStageManifestVersion,
		JobID:     strings.TrimSpace(jobID),
		Source:    strings.TrimSpace(source),
		UpdatedAt: time.Now().UTC(),
		Stages:    map[string]importStageCheckpoint{},
	}
}

func importStageManifestPath(stageDir string) string {
	return filepath.Join(strings.TrimSpace(stageDir), importStageManifestName)
}

func loadImportStageManifest(stageDir, jobID, source string) (importStageManifest, error) {
	path := importStageManifestPath(stageDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newImportStageManifest(jobID, source), nil
		}
		return importStageManifest{}, err
	}
	var manifest importStageManifest
	if err := decodeAuthoritativeJSON(data, &manifest, "import stage manifest"); err != nil {
		return importStageManifest{}, err
	}
	if manifest.Version != importStageManifestVersion {
		if err := validatePersistedSchemaVersion("import stage manifest", manifest.Version, importStageManifestVersion); err != nil {
			return importStageManifest{}, err
		}
		return importStageManifest{}, fmt.Errorf("unsupported import stage manifest version %d (expected %d)", manifest.Version, importStageManifestVersion)
	}
	if strings.TrimSpace(manifest.JobID) != "" && strings.TrimSpace(jobID) != "" && manifest.JobID != strings.TrimSpace(jobID) {
		return importStageManifest{}, errors.New("import stage manifest job id does not match")
	}
	if strings.TrimSpace(manifest.Source) != "" && strings.TrimSpace(source) != "" && manifest.Source != strings.TrimSpace(source) {
		return importStageManifest{}, errors.New("import stage manifest source does not match")
	}
	if manifest.Stages == nil {
		manifest.Stages = map[string]importStageCheckpoint{}
	}
	return manifest, nil
}

func saveImportStageManifest(stageDir string, manifest importStageManifest) error {
	if strings.TrimSpace(stageDir) == "" {
		return errors.New("stage directory is required")
	}
	manifest.Version = importStageManifestVersion
	manifest.UpdatedAt = time.Now().UTC()
	if manifest.Stages == nil {
		manifest.Stages = map[string]importStageCheckpoint{}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicFile(importStageManifestPath(stageDir), data, 0o600)
}

func saveImportStageFinalizationReplay(stageDir string, manifest *importStageManifest, replay importStageFinalizationReplay) error {
	if manifest == nil {
		return errors.New("import stage manifest is required")
	}
	manifest.Finalization = &replay
	return saveImportStageManifest(stageDir, *manifest)
}

func importStageFinalizationReplayForJob(stageDir, libraryRoot string, manifest importStageManifest) (importStageFinalizationReplay, bool) {
	if manifest.Finalization == nil {
		return importStageFinalizationReplay{}, false
	}
	replay := *manifest.Finalization
	if err := validateImportStageFinalizationReplay(stageDir, libraryRoot, replay); err != nil {
		logEvent("import_finalization_replay_rejected", "stage_dir", stageDir, "error", err.Error())
		return importStageFinalizationReplay{}, false
	}
	return replay, true
}

func validateImportStageFinalizationReplay(stageDir, libraryRoot string, replay importStageFinalizationReplay) error {
	stageDir, err := filepath.Abs(strings.TrimSpace(stageDir))
	if err != nil {
		return fmt.Errorf("resolve stage directory: %w", err)
	}
	libraryRoot, err = filepath.Abs(strings.TrimSpace(libraryRoot))
	if err != nil {
		return fmt.Errorf("resolve library root: %w", err)
	}
	if strings.TrimSpace(replay.AudioSourcePath) == "" {
		return errors.New("audio replay source is required")
	}
	if !pathWithinRoot(replay.AudioSourcePath, stageDir) || !cacheArtifactUsable(replay.AudioSourcePath, "") {
		return errors.New("audio replay source is not a usable staged artifact")
	}
	if strings.TrimSpace(replay.OriginalAudioSourcePath) != "" &&
		(!pathWithinRoot(replay.OriginalAudioSourcePath, stageDir) || !cacheArtifactUsable(replay.OriginalAudioSourcePath, "")) {
		return errors.New("original audio replay source is not a usable staged artifact")
	}
	targets := []string{
		replay.Plan.AlbumDir,
		replay.Plan.AudioPath,
		replay.Plan.LyricsPath,
		replay.Plan.LRCPath,
		replay.Plan.ArtworkPath,
		replay.Plan.VideoPath,
		replay.Plan.MetadataPath,
		replay.Metadata.Audio.OriginalPath,
	}
	for _, target := range targets {
		if strings.TrimSpace(target) == "" {
			continue
		}
		if !pathWithinRoot(target, libraryRoot) {
			return fmt.Errorf("finalization target %q is outside the library root", target)
		}
	}
	if strings.TrimSpace(replay.Metadata.Video.Path) != "" && !cacheArtifactUsable(replay.Metadata.Video.Path, "") {
		return errors.New("video replay artifact is missing or unusable")
	}
	if strings.TrimSpace(replay.Metadata.Artwork.Path) != "" && !cacheArtifactUsable(replay.Metadata.Artwork.Path, "") {
		return errors.New("artwork replay artifact is missing or unusable")
	}
	return nil
}

func checkpointImportStage(stageDir string, manifest *importStageManifest, stage, status, detail string, artifacts ...string) error {
	if manifest == nil {
		return errors.New("import stage manifest is required")
	}
	stage = strings.TrimSpace(stage)
	if stage == "" {
		return errors.New("import stage is required")
	}
	if manifest.Stages == nil {
		manifest.Stages = map[string]importStageCheckpoint{}
	}
	checkpoint := manifest.Stages[stage]
	checkpoint.Stage = stage
	checkpoint.Status = strings.TrimSpace(status)
	checkpoint.Artifacts = uniqueStrings(artifacts)
	if checkpoint.Status == "running" {
		checkpoint.Attempt++
		checkpoint.StartedAt = time.Now().UTC()
		checkpoint.CompletedAt = time.Time{}
		checkpoint.Error = ""
	} else if checkpoint.Status == "completed" {
		checkpoint.CompletedAt = time.Now().UTC()
		checkpoint.Error = ""
		manifest.LastCompleted = stage
	} else if checkpoint.Status == "failed" || checkpoint.Status == "stopped" {
		checkpoint.Error = strings.TrimSpace(detail)
	}
	manifest.Stages[stage] = checkpoint
	return saveImportStageManifest(stageDir, *manifest)
}

func importStageArtifactPathsUsable(stageDir string, checkpoint importStageCheckpoint) bool {
	for _, artifact := range checkpoint.Artifacts {
		artifact = strings.TrimSpace(artifact)
		if artifact == "" {
			continue
		}
		if !pathWithinRoot(artifact, stageDir) || !cacheArtifactUsable(artifact, "") {
			return false
		}
	}
	return checkpoint.Status == "completed"
}

// importStageRecoveryPlanForJob returns the earliest stage whose durable
// result cannot be reused. The ordering is intentionally explicit: a map's
// iteration order must never decide which work is repeated after a failure.
// Stages without persisted artifacts are not considered reusable; rerunning
// from that stage is the safe choice until their result has its own durable
// representation.
func importStageRecoveryPlanForJob(stageDir string, manifest importStageManifest, jobKind string, downloadVideo bool) importStageRecoveryPlan {
	stages := []string{importStageAudio, importStageMetadata, importStageLyrics, importStageArtwork, importStageFinalize}
	if jobKind == "url" {
		stages = []string{importStageDownload, importStageMetadata, importStageLyrics, importStageArtwork, importStageFinalize}
		if downloadVideo {
			stages = []string{importStageDownload, importStageMetadata, importStageLyrics, importStageVideo, importStageArtwork, importStageFinalize}
		}
	}

	plan := importStageRecoveryPlan{}
	for index, stage := range stages {
		checkpoint, ok := manifest.Stages[stage]
		if ok && stage == importStageFinalize && checkpoint.Status == "completed" {
			// A crash can occur after the durable catalog write and before the
			// caller removes Incoming/<job-id>. Treat the complete manifest as
			// reusable evidence so a restart does not invoke yt-dlp again or
			// duplicate the catalog entry.
			plan.ReusableStages = append([]string(nil), stages...)
			return plan
		}
		if !ok || !importStageArtifactPathsUsable(stageDir, checkpoint) {
			plan.RetryFrom = stage
			plan.InvalidatedStages = append([]string(nil), stages[index:]...)
			return plan
		}
		plan.ReusableStages = append(plan.ReusableStages, stage)
	}
	return plan
}

func (plan importStageRecoveryPlan) canReuse(stage string) bool {
	for _, reusable := range plan.ReusableStages {
		if reusable == stage {
			return true
		}
	}
	return false
}
