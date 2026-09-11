package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"melodex/internal/songstore"
)

// finalizeImportFromReplay is the single finalization path for both a first
// attempt and a recovered attempt. Keeping the write, track construction,
// catalog upsert, and final checkpoint together makes each retry idempotent at
// the durable boundary.
func (a *App) finalizeImportFromReplay(jobID, stageDir string, manifest *importStageManifest, replay importStageFinalizationReplay) error {
	if a == nil || a.store == nil {
		return errors.New("app store is required for import finalization")
	}
	if manifest == nil {
		return errors.New("import stage manifest is required")
	}
	if err := validateImportStageFinalizationReplay(stageDir, a.info.LibraryRoot, replay); err != nil {
		return err
	}
	probe, err := probeAudioFile(replay.AudioSourcePath, a.toolPath(a.settings.FFmpegPath, "ffmpeg"))
	if err != nil {
		_ = checkpointImportStage(stageDir, manifest, importStageFinalize, "failed", "Audio validation failed: "+err.Error())
		return fmt.Errorf("validate staged audio: %w", err)
	}
	applyAudioProbe(&replay.Metadata, probe)

	if replay.Metadata.TrackID == "" {
		replay.Metadata.TrackID = songstore.TrackIDFromPath(replay.Plan.MetadataPath)
	}
	if err := checkpointImportStage(stageDir, manifest, importStageFinalize, "running", "Writing catalog files", replay.Plan.AudioPath, replay.Plan.MetadataPath); err != nil {
		return err
	}
	if err := songstore.WriteSongFiles(
		replay.Plan,
		replay.AudioSourcePath,
		replay.OriginalAudioSourcePath,
		replay.Lyrics,
		replay.Metadata,
	); err != nil {
		if checkpointErr := checkpointImportStage(stageDir, manifest, importStageFinalize, "failed", err.Error()); checkpointErr != nil {
			return fmt.Errorf("write song files: %w (checkpoint failed: %v)", err, checkpointErr)
		}
		return err
	}

	record, err := buildTrackRecord(replay.Plan, replay.Metadata, replay.SourceKind, replay.SourceRef, replay.AudioSourcePath)
	if err != nil {
		if checkpointErr := checkpointImportStage(stageDir, manifest, importStageFinalize, "failed", err.Error()); checkpointErr != nil {
			return fmt.Errorf("build track record: %w (checkpoint failed: %v)", err, checkpointErr)
		}
		return err
	}
	record = a.trackWithArtworkMediaURL(record)
	a.upsertImportedTrack(record)
	if err := checkpointImportStage(stageDir, manifest, importStageFinalize, "completed", "Catalog files durable", replay.Plan.MetadataPath, replay.Plan.AudioPath); err != nil {
		return err
	}
	a.setJobResult(jobID, replay.Metadata.Title, replay.Metadata.Artist, replay.Metadata.Album)
	return nil
}

// upsertImportedTrack prevents a crash between catalog persistence and the
// final stage checkpoint from creating a duplicate on replay. Matching uses
// only durable identities already present in the track record: ID, metadata
// path, or audio path. It does not infer a broader duplicate policy.
func (a *App) upsertImportedTrack(track TrackRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for index := range a.catalog.Tracks {
		if importedTrackIdentityMatches(a.catalog.Tracks[index], track) {
			a.catalog.Tracks[index] = track
			a.rebuildTrackIndexLocked()
			a.persistLocked()
			a.emitStateLocked(a.snapshotLocked())
			log.Printf("import finalization upserted existing track: %s", firstNonEmpty(track.ID, track.MetadataPath, track.AudioPath))
			logEvent("import_track_upserted", "track_id", track.ID, "metadata_path", track.MetadataPath, "audio_path", track.AudioPath, "replaced", true)
			return
		}
	}
	a.catalog.Tracks = append([]TrackRecord{track}, a.catalog.Tracks...)
	a.rebuildTrackIndexLocked()
	a.persistLocked()
	a.emitStateLocked(a.snapshotLocked())
	logEvent("import_track_upserted", "track_id", track.ID, "metadata_path", track.MetadataPath, "audio_path", track.AudioPath, "replaced", false)
}

func importedTrackIdentityMatches(existing, incoming TrackRecord) bool {
	if strings.TrimSpace(existing.ID) != "" && strings.TrimSpace(incoming.ID) != "" && existing.ID == incoming.ID {
		return true
	}
	for _, paths := range [][2]string{
		{existing.MetadataPath, incoming.MetadataPath},
		{existing.AudioPath, incoming.AudioPath},
	} {
		left := strings.TrimSpace(paths[0])
		right := strings.TrimSpace(paths[1])
		if left != "" && right != "" && filepath.Clean(left) == filepath.Clean(right) {
			return true
		}
	}
	return false
}

func importFinalizationReplayForJob(a *App, stageDir string, manifest importStageManifest) (importStageFinalizationReplay, bool) {
	if a == nil {
		return importStageFinalizationReplay{}, false
	}
	replay, ok := importStageFinalizationReplayForJob(stageDir, a.info.LibraryRoot, manifest)
	if !ok {
		return importStageFinalizationReplay{}, false
	}
	if _, err := os.Stat(replay.AudioSourcePath); err != nil {
		logEvent("import_finalization_replay_rejected", "stage_dir", stageDir, "reason", "audio_source_missing", "error", err.Error())
		return importStageFinalizationReplay{}, false
	}
	return replay, true
}
