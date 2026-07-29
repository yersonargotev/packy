package capabilitypack

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
)

type FileActivationStore struct {
	path string
	mu   sync.Mutex
}

type activationDocument struct {
	SchemaVersion int               `json:"schema_version"`
	Activations   []ActivationState `json:"activations"`
}

func NewFileActivationStore(path string) *FileActivationStore {
	return &FileActivationStore{path: path}
}

func (s *FileActivationStore) Load(_ context.Context, surface Surface) (ActivationState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	document, err := s.load()
	if err != nil {
		return ActivationState{}, err
	}
	return activationForSurface(document, surface), nil
}

// Save compares the durable revision for one surface and atomically replaces
// the whole document, preserving every other surface's intent and ownership.
func (s *FileActivationStore) Save(_ context.Context, surface Surface, expectedRevision int, state ActivationState) error {
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
	document, err := s.load()
	if err != nil {
		return err
	}
	current := activationForSurface(document, surface)
	if current.Intent.Revision != expectedRevision {
		return StalePlanError{Precondition: fmt.Sprintf("activation intent revision changed from %d to %d before persistence; rerun activation to preview a fresh plan", expectedRevision, current.Intent.Revision)}
	}
	state.SchemaVersion = 3
	state.Intent.Surface = surface
	if err := canonicalizeActivationState(&state); err != nil {
		return err
	}
	replaced := false
	for i := range document.Activations {
		if document.Activations[i].Intent.Surface == surface {
			document.Activations[i] = cloneActivationState(state)
			replaced = true
			break
		}
	}
	if !replaced {
		document.Activations = append(document.Activations, cloneActivationState(state))
	}
	sort.Slice(document.Activations, func(i, j int) bool {
		return document.Activations[i].Intent.Surface < document.Activations[j].Intent.Surface
	})
	document.SchemaVersion = 4
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode capability-pack state: %w", err)
	}
	return atomicWriteState(s.path, append(data, '\n'))
}

func activationForSurface(document activationDocument, surface Surface) ActivationState {
	for _, state := range document.Activations {
		if state.Intent.Surface == surface {
			return cloneActivationState(state)
		}
	}
	return ActivationState{}
}

func (s *FileActivationStore) load() (activationDocument, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return activationDocument{}, nil
	}
	if err != nil {
		return activationDocument{}, fmt.Errorf("read capability-pack state %s: %w", s.path, err)
	}
	var header struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return activationDocument{}, fmt.Errorf("read capability-pack state %s: invalid JSON: %w", s.path, err)
	}
	switch header.SchemaVersion {
	case 1:
		var legacy ActivationState
		if err := json.Unmarshal(data, &legacy); err != nil {
			return activationDocument{}, err
		}
		document := activationDocument{SchemaVersion: 4, Activations: []ActivationState{legacy}}
		defaultLegacySelections(&document)
		if err := canonicalizeActivationDocument(&document); err != nil {
			return activationDocument{}, err
		}
		return document, nil
	case 2:
		var document activationDocument
		if err := json.Unmarshal(data, &document); err != nil {
			return activationDocument{}, err
		}
		for _, state := range document.Activations {
			if state.SchemaVersion != 1 {
				return activationDocument{}, fmt.Errorf("read capability-pack state %s: document v2 contains unsupported activation schema_version %d", s.path, state.SchemaVersion)
			}
		}
		defaultLegacySelections(&document)
		if err := canonicalizeActivationDocument(&document); err != nil {
			return activationDocument{}, err
		}
		return document, nil
	case 3:
		var document activationDocument
		if err := json.Unmarshal(data, &document); err != nil {
			return activationDocument{}, err
		}
		for _, state := range document.Activations {
			if state.SchemaVersion != 2 {
				return activationDocument{}, fmt.Errorf("read capability-pack state %s: document v3 contains unsupported activation schema_version %d", s.path, state.SchemaVersion)
			}
		}
		defaultLegacySelections(&document)
		if err := canonicalizeActivationDocument(&document); err != nil {
			return activationDocument{}, err
		}
		return document, nil
	case 4:
		var document activationDocument
		if err := strictDecode(data, &document); err != nil {
			return activationDocument{}, fmt.Errorf("read capability-pack state %s: invalid document v4: %w", s.path, err)
		}
		for _, state := range document.Activations {
			if state.SchemaVersion != 3 {
				return activationDocument{}, fmt.Errorf("read capability-pack state %s: document v4 contains unsupported activation schema_version %d", s.path, state.SchemaVersion)
			}
			if err := validateExplicitSelections(state); err != nil {
				return activationDocument{}, fmt.Errorf("read capability-pack state %s: %w", s.path, err)
			}
		}
		if err := canonicalizeActivationDocument(&document); err != nil {
			return activationDocument{}, err
		}
		return document, nil
	default:
		return activationDocument{}, fmt.Errorf("read capability-pack state %s: unsupported schema_version %d", s.path, header.SchemaVersion)
	}
}

func canonicalizeActivationDocument(document *activationDocument) error {
	document.SchemaVersion = 4
	for i := range document.Activations {
		if err := canonicalizeActivationState(&document.Activations[i]); err != nil {
			return err
		}
	}
	return nil
}

func canonicalizeActivationState(state *ActivationState) error {
	state.SchemaVersion = 3
	var err error
	state.Intent.Selection, err = canonicalSelection(state.Intent.Selection)
	if err != nil {
		return err
	}
	if err := canonicalizeAliases(&state.Intent.Aliases); err != nil {
		return err
	}
	state.Intent.ProviderChoices, err = canonicalProviderChoices(state.Intent.ProviderChoices)
	if err != nil {
		return err
	}
	for i := range state.Intents {
		state.Intents[i].Selection, err = canonicalSelection(state.Intents[i].Selection)
		if err != nil {
			return err
		}
		if err := canonicalizeAliases(&state.Intents[i].Aliases); err != nil {
			return err
		}
		state.Intents[i].ProviderChoices, err = canonicalProviderChoices(state.Intents[i].ProviderChoices)
		if err != nil {
			return err
		}
	}
	return nil
}

func defaultLegacySelections(document *activationDocument) {
	all := ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}
	for i := range document.Activations {
		document.Activations[i].Intent.Selection = all
		for j := range document.Activations[i].Intents {
			document.Activations[i].Intents[j].Selection = all
		}
	}
}

func validateExplicitSelections(state ActivationState) error {
	intents := append([]ActivationIntent{state.Intent}, state.Intents...)
	for _, intent := range intents {
		if intent.Selection.Mode == "" {
			return fmt.Errorf("activation intent for pack %q is missing explicit resource selection", intent.PackID)
		}
		if _, err := canonicalSelection(intent.Selection); err != nil {
			return fmt.Errorf("activation intent for pack %q has invalid resource selection: %w", intent.PackID, err)
		}
	}
	return nil
}

func canonicalizeAliases(aliases *[]SurfaceAlias) error {
	if *aliases == nil {
		*aliases = []SurfaceAlias{}
	}
	seen := map[string]bool{}
	for _, alias := range *aliases {
		if alias.Kind != "skill" && alias.Kind != "agent" && alias.Kind != "command" {
			return fmt.Errorf("activation alias kind %q is unsupported", alias.Kind)
		}
		if !idPattern.MatchString(alias.ID) {
			return fmt.Errorf("activation alias id %q is invalid", alias.ID)
		}
		if alias.Name == "" || strings.TrimSpace(alias.Name) != alias.Name {
			return fmt.Errorf("activation alias name must be nonempty canonical text")
		}
		key := alias.Kind + ":" + alias.ID
		if seen[key] {
			return fmt.Errorf("activation alias identity %q is duplicated", key)
		}
		seen[key] = true
	}
	sort.Slice(*aliases, func(i, j int) bool {
		if (*aliases)[i].Kind != (*aliases)[j].Kind {
			return (*aliases)[i].Kind < (*aliases)[j].Kind
		}
		return (*aliases)[i].ID < (*aliases)[j].ID
	})
	return nil
}

func atomicWriteState(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".packs-*.tmp")
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
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace capability-pack state: %w", err)
	}
	return nil
}
