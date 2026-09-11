package songstore

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

type atomicFileBackup struct {
	target  string
	backup  string
	existed bool
}

type atomicFileBackups []atomicFileBackup

// snapshotAtomicTargets records the files that a multi-file import is about
// to replace. Individual writes are atomic; the snapshots add rollback for a
// later sidecar failure so an existing audio/lyrics/metadata set is not
// deleted or left partially updated.
func snapshotAtomicTargets(paths []string) (atomicFileBackups, error) {
	backups := make(atomicFileBackups, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	cleanup := func() {
		for _, backup := range backups {
			if backup.backup != "" {
				_ = os.Remove(backup.backup)
			}
		}
	}
	for _, path := range paths {
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			backups = append(backups, atomicFileBackup{target: path})
			continue
		}
		if err != nil {
			cleanup()
			return nil, err
		}
		if !info.Mode().IsRegular() {
			cleanup()
			return nil, os.ErrInvalid
		}
		backupFile, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".backup-*")
		if err != nil {
			cleanup()
			return nil, err
		}
		backupPath := backupFile.Name()
		ok := false
		sourceFile, sourceErr := os.Open(path)
		if sourceErr == nil {
			_, sourceErr = io.Copy(backupFile, sourceFile)
			_ = sourceFile.Close()
		}
		if sourceErr == nil {
			if err := backupFile.Chmod(info.Mode().Perm()); err == nil {
				err = backupFile.Sync()
			}
			if err == nil {
				err = backupFile.Close()
				ok = err == nil
			}
		}
		if !ok {
			_ = backupFile.Close()
			_ = os.Remove(backupPath)
			cleanup()
			return nil, errors.New("failed to snapshot existing persistence file")
		}
		backups = append(backups, atomicFileBackup{target: path, backup: backupPath, existed: true})
	}
	return backups, nil
}

func (backups atomicFileBackups) commit() {
	for _, backup := range backups {
		if backup.backup != "" {
			_ = os.Remove(backup.backup)
		}
	}
}

func (backups atomicFileBackups) rollback() error {
	var firstErr error
	for i := len(backups) - 1; i >= 0; i-- {
		backup := backups[i]
		if err := os.Remove(backup.target); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
			firstErr = err
		}
		if backup.existed {
			if err := os.Rename(backup.backup, backup.target); err != nil && firstErr == nil {
				firstErr = err
			}
			backup.backup = ""
		}
		if backup.backup != "" {
			_ = os.Remove(backup.backup)
		}
	}
	return firstErr
}
