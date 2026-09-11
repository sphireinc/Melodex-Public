package songstore

import "sync"

type atomicWriteStage uint8

const (
	atomicWriteStageWrite atomicWriteStage = iota + 1
	atomicWriteStageRename
)

// The hook is nil during normal execution. Tests use it to model a failed
// write or rename without depending on host permissions, disk quotas, or
// platform-specific filesystem behavior.
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
