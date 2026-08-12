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
	SchemaVersion int                   `json:"schema_version"`
	Revision      int                   `json:"revision"`
	Activations   []ActivationState     `json:"activations"`
	Ownership     []ProjectionOwnership `json:"ownership,omitempty"`
}

type installedPackIdentity struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type installedProjection struct {
	ID       string `json:"id"`
	Target   string `json:"target"`
	Digest   string `json:"digest"`
	FileMode uint32 `json:"file_mode,omitempty"`
}

type installedPackReceipt struct {
	Pack                 installedPackIdentity        `json:"pack"`
	Surface              Surface                      `json:"surface"`
	ReadinessObligations []ReadinessObligation        `json:"readiness_obligations"`
	ExternalRequirements []string                     `json:"external_requirements"`
	Selection            ResourceSelection            `json:"selection"`
	Aliases              []SurfaceAlias               `json:"aliases,omitempty"`
	Resources            []ResourceIdentity           `json:"resources"`
	Projections          []installedProjection        `json:"projections"`
	Sensitive            []ProjectSensitiveDisclosure `json:"sensitive,omitempty"`
}

type installedReceiptDocument struct {
	SchemaVersion int                    `json:"schema_version"`
	Revision      int                    `json:"revision"`
	Receipts      []installedPackReceipt `json:"receipts"`
}

func NewFileActivationStore(path string) *FileActivationStore {
	return &FileActivationStore{path: path}
}

func (s *FileActivationStore) LoadSnapshot(_ context.Context, surface Surface) (ActivationState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	document, err := s.load()
	if err != nil {
		return ActivationState{}, err
	}
	return snapshotStateForSurface(document, surface), nil
}

func (s *FileActivationStore) SaveSnapshot(_ context.Context, surface Surface, expectedDocumentRevision int, state ActivationState) (int, error) {
	return s.save(surface, expectedDocumentRevision, state.Intent.Revision, state, true)
}

func (s *FileActivationStore) save(surface Surface, expectedDocumentRevision, expectedIntentRevision int, state ActivationState, compareDocument bool) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return 0, fmt.Errorf("create capability-pack state directory: %w", err)
	}
	lock, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open capability-pack state lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return 0, fmt.Errorf("lock capability-pack state: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	document, err := s.load()
	if err != nil {
		return 0, err
	}
	if compareDocument && document.Revision != expectedDocumentRevision {
		return document.Revision, StalePlanError{Precondition: fmt.Sprintf("capability-pack state revision changed from %d to %d before persistence; rerun activation to preview a fresh plan", expectedDocumentRevision, document.Revision)}
	}
	current := activationForSurface(document, surface)
	if !compareDocument && current.Intent.Revision != expectedIntentRevision {
		return document.Revision, StalePlanError{Precondition: fmt.Sprintf("activation intent revision changed from %d to %d before persistence; rerun activation to preview a fresh plan", expectedIntentRevision, current.Intent.Revision)}
	}
	state.SchemaVersion = 3
	state.Intent.Surface = surface
	if err := canonicalizeActivationState(&state); err != nil {
		return document.Revision, err
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
	document.SchemaVersion = 1
	document.Revision++
	document.Ownership = cloneOwnership(state.Ownership)
	for i := range document.Activations {
		document.Activations[i].Ownership = nil
	}
	data, err := json.MarshalIndent(receiptDocumentFromActivation(document), "", "  ")
	if err != nil {
		return document.Revision, fmt.Errorf("encode capability-pack state: %w", err)
	}
	if err := atomicWriteState(s.path, append(data, '\n')); err != nil {
		return document.Revision, err
	}
	return document.Revision, nil
}

func snapshotStateForSurface(document activationDocument, surface Surface) ActivationState {
	state := activationForSurface(document, surface)
	state.Ownership = cloneOwnership(document.Ownership)
	state.documentRevision = document.Revision
	state.snapshotManaged = true
	return state
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
	var receipts installedReceiptDocument
	if err := strictDecode(data, &receipts); err != nil {
		var fields map[string]json.RawMessage
		if json.Unmarshal(data, &fields) == nil && (fields["activations"] != nil || fields["ownership"] != nil) {
			return activationDocument{}, fmt.Errorf("read capability-pack state %s: unsupported legacy capability-pack state; reset Packy state before using v0.2", s.path)
		}
		return activationDocument{}, fmt.Errorf("read capability-pack state %s: invalid installed receipt document: %w", s.path, err)
	}
	if receipts.SchemaVersion != 1 {
		return activationDocument{}, fmt.Errorf("read capability-pack state %s: unsupported schema_version %d", s.path, receipts.SchemaVersion)
	}
	document, err := activationDocumentFromReceipts(receipts)
	if err != nil {
		return activationDocument{}, fmt.Errorf("read capability-pack state %s: %w", s.path, err)
	}
	return document, nil
}

func receiptDocumentFromActivation(document activationDocument) installedReceiptDocument {
	receipts := installedReceiptDocument{SchemaVersion: 1, Revision: document.Revision, Receipts: []installedPackReceipt{}}
	for _, state := range document.Activations {
		for _, intent := range activeIntents(state) {
			if !intent.Active {
				continue
			}
			receipt := installedPackReceipt{
				Pack: installedPackIdentity{ID: intent.PackID, Version: intent.Version}, Surface: intent.Surface,
				ReadinessObligations: append([]ReadinessObligation(nil), intent.ReadinessObligations...),
				ExternalRequirements: append([]string{}, intent.ExternalRequirements...),
				Selection:            cloneSelection(intent.Selection), Aliases: cloneAliases(intent.Aliases),
				Resources: append([]ResourceIdentity(nil), intent.Resources...),
			}
			for _, owner := range document.Ownership {
				if !ownershipBelongsToReceipt(owner, intent.PackID, intent.Surface) {
					continue
				}
				receipt.Projections = append(receipt.Projections, installedProjection{
					ID: owner.ProjectionID, Target: owner.Target, Digest: owner.Fingerprint,
				})
			}
			sort.Slice(receipt.Resources, func(i, j int) bool { return receipt.Resources[i].String() < receipt.Resources[j].String() })
			sort.Slice(receipt.Projections, func(i, j int) bool { return receipt.Projections[i].Target < receipt.Projections[j].Target })
			receipts.Receipts = append(receipts.Receipts, receipt)
		}
	}
	sort.Slice(receipts.Receipts, func(i, j int) bool {
		if receipts.Receipts[i].Pack.ID != receipts.Receipts[j].Pack.ID {
			return receipts.Receipts[i].Pack.ID < receipts.Receipts[j].Pack.ID
		}
		return receipts.Receipts[i].Surface < receipts.Receipts[j].Surface
	})
	return receipts
}

func activationDocumentFromReceipts(receipts installedReceiptDocument) (activationDocument, error) {
	document := activationDocument{SchemaVersion: 1, Revision: receipts.Revision, Activations: []ActivationState{}, Ownership: []ProjectionOwnership{}}
	bySurface := map[Surface]*ActivationState{}
	owners := map[string]ProjectionOwnership{}
	for _, receipt := range receipts.Receipts {
		if receipt.Pack.ID == "" || receipt.Pack.Version == "" || receipt.Surface == "" {
			return activationDocument{}, fmt.Errorf("installed receipt requires Pack identity, version, and surface")
		}
		state := bySurface[receipt.Surface]
		if state == nil {
			state = &ActivationState{SchemaVersion: 3}
			bySurface[receipt.Surface] = state
		}
		explicit := true
		intent := ActivationIntent{PackID: receipt.Pack.ID, Version: receipt.Pack.Version, Surface: receipt.Surface, Active: true, Revision: receipts.Revision, ReadinessObligations: append([]ReadinessObligation(nil), receipt.ReadinessObligations...), ExternalRequirements: append([]string{}, receipt.ExternalRequirements...), Aliases: cloneAliases(receipt.Aliases), Selection: cloneSelection(receipt.Selection), Resources: append([]ResourceIdentity(nil), receipt.Resources...), Explicit: &explicit}
		state.Intents = append(state.Intents, intent)
		state.Intent = intent
		for _, projection := range receipt.Projections {
			owner := ProjectionOwnership{
				ID: receiptProjectionOwnershipID(receipt.Surface, projection), ProjectionID: projection.ID, Target: projection.Target, Fingerprint: projection.Digest,
				PackID: receipt.Pack.ID, Surface: receipt.Surface,
			}
			if _, exists := owners[owner.ID]; exists {
				return activationDocument{}, fmt.Errorf("installed receipts collide at projection %q", owner.ID)
			}
			owners[owner.ID] = owner
		}
	}
	for _, state := range bySurface {
		sort.Slice(state.Intents, func(i, j int) bool { return state.Intents[i].PackID < state.Intents[j].PackID })
		document.Activations = append(document.Activations, *state)
	}
	for _, owner := range owners {
		document.Ownership = append(document.Ownership, owner)
	}
	if err := canonicalizeActivationDocument(&document); err != nil {
		return activationDocument{}, err
	}
	return document, nil
}

func receiptProjectionOwnershipID(surface Surface, projection installedProjection) string {
	if (surface == SurfaceCodex || surface == SurfaceOpenCode) && strings.HasPrefix(projection.ID, "skill:") && projection.Target != "" {
		return "path:" + filepath.Clean(projection.Target)
	}
	return "surface:" + string(surface) + ":" + projection.ID
}

func ownershipBelongsToReceipt(owner ProjectionOwnership, packID string, surface Surface) bool {
	return owner.PackID == packID && owner.Surface == surface
}

func canonicalizeActivationDocument(document *activationDocument) error {
	document.SchemaVersion = 1
	for i := range document.Activations {
		if err := canonicalizeActivationState(&document.Activations[i]); err != nil {
			return err
		}
	}
	sort.Slice(document.Ownership, func(i, j int) bool { return document.Ownership[i].ID < document.Ownership[j].ID })
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
	for i := range state.Intents {
		state.Intents[i].Selection, err = canonicalSelection(state.Intents[i].Selection)
		if err != nil {
			return err
		}
		if err := canonicalizeAliases(&state.Intents[i].Aliases); err != nil {
			return err
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
