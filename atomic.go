package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type atomicWriteStage uint8

const (
	atomicWriteStageWrite atomicWriteStage = iota + 1
	atomicWriteStageRename
)

// The hook is nil during normal application execution. It exists so
// persistence tests can deterministically exercise failures at the two
// boundaries where an atomic write must not disturb the destination file.
var atomicWriteFailureState struct {
	sync.RWMutex
	hook func(atomicWriteStage, string) error
}

func installAtomicWriteFailureHook(hook func(atomicWriteStage, string) error) func() {
	atomicWriteFailureState.Lock()
	previous := atomicWriteFailureState.hook
	atomicWriteFailureState.hook = hook
	atomicWriteFailureState.Unlock()
	return func() {
		atomicWriteFailureState.Lock()
		atomicWriteFailureState.hook = previous
		atomicWriteFailureState.Unlock()
	}
}

func atomicWriteFailure(stage atomicWriteStage, path string) error {
	atomicWriteFailureState.RLock()
	hook := atomicWriteFailureState.hook
	atomicWriteFailureState.RUnlock()
	if hook == nil {
		return nil
	}
	return hook(stage, path)
}

func writeAtomicFile(path string, data []byte, perm os.FileMode) error {
	return writeAtomicStream(path, bytes.NewReader(data), perm)
}

func writeAtomicStream(path string, r io.Reader, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if err := atomicWriteFailure(atomicWriteStageWrite, path); err != nil {
		cleanup()
		return err
	}
	if _, err := io.Copy(tmp, r); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := commitAtomicTempFile(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func commitAtomicTempFile(tmpPath, path string) error {
	if err := atomicWriteFailure(atomicWriteStageRename, path); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}
