package capabilitypack

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

type FileActivationStore struct {
	path string
	mu   sync.Mutex
}

func NewFileActivationStore(path string) *FileActivationStore {
	return &FileActivationStore{path: path}
}

func (s *FileActivationStore) Load(context.Context) (ActivationState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// Save compares the durable intent revision and atomically replaces the whole
// activation document, keeping target intent and the applying journal together.
func (s *FileActivationStore) Save(_ context.Context, expectedRevision int, state ActivationState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create capability-pack state directory: %w", err)
	}
	lock, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open capability-pack state lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock capability-pack state: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	current, err := s.load()
	if err != nil {
		return err
	}
	if current.Intent.Revision != expectedRevision {
		return StalePlanError{Precondition: fmt.Sprintf("activation intent revision changed from %d to %d before persistence; rerun activation to preview a fresh plan", expectedRevision, current.Intent.Revision)}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode capability-pack state: %w", err)
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(s.path), ".packs-*.tmp")
	if err != nil {
		return fmt.Errorf("create capability-pack state temp file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write capability-pack state: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync capability-pack state: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close capability-pack state: %w", err)
	}
	if err := os.Rename(tempPath, s.path); err != nil {
		return fmt.Errorf("replace capability-pack state: %w", err)
	}
	return nil
}

func (s *FileActivationStore) load() (ActivationState, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return ActivationState{}, nil
	}
	if err != nil {
		return ActivationState{}, fmt.Errorf("read capability-pack state %s: %w", s.path, err)
	}
	var state ActivationState
	if err := json.Unmarshal(data, &state); err != nil {
		return ActivationState{}, fmt.Errorf("read capability-pack state %s: invalid JSON: %w", s.path, err)
	}
	if state.SchemaVersion != 1 {
		return ActivationState{}, fmt.Errorf("read capability-pack state %s: unsupported schema_version %d", s.path, state.SchemaVersion)
	}
	return state, nil
}
