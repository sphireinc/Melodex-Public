package songstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSidecarAndArtworkWritersPreserveExistingFilesOnAtomicFailure(t *testing.T) {
	cases := []struct {
		name   string
		write  func(string) error
		old    []byte
		prefix string
	}{
		{name: "plain lyrics", write: func(path string) error { return WritePlainLyrics(path, "new lyrics") }, old: []byte("old lyrics"), prefix: "lyrics"},
		{name: "timed lyrics", write: func(path string) error { return WriteLRC(path, []TimedLyricLine{{StartSeconds: 1, Text: "new"}}) }, old: []byte("[00:01.00]old\n"), prefix: "lyrics"},
		{name: "metadata sidecar", write: func(path string) error { return WriteMetadataJSON(path, SongMetadata{Title: "new"}) }, old: []byte("old metadata"), prefix: "metadata"},
		{name: "artwork", write: func(path string) error { return WriteBinaryFile(path, []byte("new artwork")) }, old: []byte("old artwork"), prefix: "cover"},
	}
	for _, stageCase := range []struct {
		name  string
		stage atomicWriteStage
	}{
		{name: "write", stage: atomicWriteStageWrite},
		{name: "rename", stage: atomicWriteStageRename},
	} {
		t.Run(stageCase.name, func(t *testing.T) {
			injected := errors.New("injected " + stageCase.name + " failure")
			restore := installAtomicWriteFailureHook(func(stage atomicWriteStage, _ string) error {
				if stage == stageCase.stage {
					return injected
				}
				return nil
			})
			t.Cleanup(restore)

			for _, writerCase := range cases {
				t.Run(writerCase.name, func(t *testing.T) {
					path := filepath.Join(t.TempDir(), writerCase.prefix+".fixture")
					if err := os.WriteFile(path, writerCase.old, 0o600); err != nil {
						t.Fatalf("write fixture: %v", err)
					}
					if err := writerCase.write(path); !errors.Is(err, injected) {
						t.Fatalf("expected injected error, got %v", err)
					}
					assertSongstoreFileBytes(t, path, writerCase.old)
					assertNoSongstoreTempFiles(t, path)
				})
			}
		})
	}
}

func TestWriteSongFilesRollsBackExistingSidecarsOnLaterFailure(t *testing.T) {
	root := t.TempDir()
	plan, err := BuildSongStoragePlan(root, SongMetadata{Title: "Song", Artist: "Artist", Album: "Album"})
	if err != nil {
		t.Fatalf("BuildSongStoragePlan: %v", err)
	}
	oldAudio := filepath.Join(root, "old.mp3")
	if err := os.WriteFile(oldAudio, []byte("old audio"), 0o600); err != nil {
		t.Fatalf("write old audio: %v", err)
	}
	oldLyrics := LyricsResult{Text: "old lyrics", TimedLyrics: []TimedLyricLine{{StartSeconds: 1, Text: "old"}}, TimingGranularity: "line"}
	if err := WriteSongFiles(plan, oldAudio, "", oldLyrics, SongMetadata{Title: "Song", Artist: "Artist", Album: "Album"}); err != nil {
		t.Fatalf("initial WriteSongFiles: %v", err)
	}
	paths := []string{plan.AudioPath, plan.LyricsPath, plan.LRCPath, plan.MetadataPath}
	previous := make(map[string][]byte, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read initial %s: %v", path, err)
		}
		previous[path] = data
	}

	injected := errors.New("injected metadata sidecar failure")
	restore := installAtomicWriteFailureHook(func(stage atomicWriteStage, path string) error {
		if stage == atomicWriteStageWrite && filepath.Clean(path) == filepath.Clean(plan.MetadataPath) {
			return injected
		}
		return nil
	})
	t.Cleanup(restore)
	newAudio := filepath.Join(root, "new.mp3")
	if err := os.WriteFile(newAudio, []byte("new audio"), 0o600); err != nil {
		t.Fatalf("write new audio: %v", err)
	}
	err = WriteSongFiles(plan, newAudio, "", LyricsResult{Text: "new lyrics"}, SongMetadata{Title: "Changed", Artist: "Artist", Album: "Album"})
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected metadata failure, got %v", err)
	}
	for path, want := range previous {
		assertSongstoreFileBytes(t, path, want)
	}
	assertNoSongstoreBackupFiles(t, plan.MetadataPath)
}

func assertSongstoreFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("file %s changed: got %q want %q", path, got, want)
	}
}

func assertNoSongstoreTempFiles(t *testing.T, path string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

func assertNoSongstoreBackupFiles(t *testing.T, path string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".*.backup-*"))
	if err != nil {
		t.Fatalf("glob backup files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("backup files remain: %v", matches)
	}
}
