package offlinevalidation

import (
	"fmt"
	"runtime/debug"
)

const (
	workerHeapBytes  = int64(512 << 20)
	workerMaxThreads = 64
	workerMaxStack   = 32 << 20
)

func installRuntimeBounds() error {
	debug.SetMemoryLimit(workerHeapBytes)
	if previous := debug.SetMemoryLimit(workerHeapBytes); previous != workerHeapBytes {
		return fmt.Errorf("verify Go heap limit: got %d, want %d", previous, workerHeapBytes)
	}
	debug.SetMaxThreads(workerMaxThreads)
	if previous := debug.SetMaxThreads(workerMaxThreads); previous != workerMaxThreads {
		return fmt.Errorf("verify Go thread limit: got %d, want %d", previous, workerMaxThreads)
	}
	debug.SetMaxStack(workerMaxStack)
	if previous := debug.SetMaxStack(workerMaxStack); previous != workerMaxStack {
		return fmt.Errorf("verify Go stack limit: got %d, want %d", previous, workerMaxStack)
	}
	return nil
}
