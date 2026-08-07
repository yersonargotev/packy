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
	External      []ExternalEffect      `json:"-"`
}

type installedPackIdentity struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type installedProjection struct {
	ID                string                `json:"id"`
	PhysicalID        string                `json:"physical_id"`
	Target            string                `json:"target"`
	Digest            string                `json:"digest"`
	Contributors      []string              `json:"contributors"`
	AdapterProvenance string                `json:"adapter_provenance,omitempty"`
	Authorities       []ProjectionAuthority `json:"authorities,omitempty"`
	Mode              string                `json:"mode,omitempty"`
	FileMode          uint32                `json:"file_mode,omitempty"`
	Command           string                `json:"command,omitempty"`
	Args              []string              `json:"args,omitempty"`
	DiscoverableBy    []Surface             `json:"discoverable_by,omitempty"`
}

type installedPackReceipt struct {
	Pack            installedPackIdentity        `json:"pack"`
	Surface         Surface                      `json:"surface"`
	Selection       ResourceSelection            `json:"selection"`
	Aliases         []SurfaceAlias               `json:"aliases,omitempty"`
	Resources       []ResourceIdentity           `json:"resources"`
	Projections     []installedProjection        `json:"projections"`
	Sensitive       []ProjectSensitiveDisclosure `json:"sensitive,omitempty"`
	ExternalEffects []ExternalEffect             `json:"external_effects,omitempty"`
}

type installedReceiptDocument struct {
	SchemaVersion   int                    `json:"schema_version"`
	Revision        int                    `json:"revision"`
	Receipts        []installedPackReceipt `json:"receipts"`
	ExternalEffects []ExternalEffect       `json:"external_effects,omitempty"`
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
	return snapshotStateForSurface(document, surface), nil
}

func (s *FileActivationStore) LoadSnapshot(ctx context.Context, surface Surface) (ActivationState, error) {
	return s.Load(ctx, surface)
}

// Save compares the durable revision for one surface and atomically replaces
// the whole document, preserving every other surface's intent and ownership.
func (s *FileActivationStore) Save(_ context.Context, surface Surface, expectedRevision int, state ActivationState) error {
	_, err := s.save(surface, state.documentRevision, expectedRevision, state, false)
	return err
}

func (s *FileActivationStore) SaveSnapshot(_ context.Context, surface Surface, expectedDocumentRevision int, state ActivationState) (int, error) {
	// Plans and attempts are deliberately memory-only. Verified external-effect
	// receipts are durable independently so a later failure cannot erase the
	// exact reversal authority for an effect that already occurred.
	if state.Journal != nil {
		return s.saveVerifiedExternalReceipts(expectedDocumentRevision, state)
	}
	return s.save(surface, expectedDocumentRevision, state.Intent.Revision, state, true)
}

func (s *FileActivationStore) saveVerifiedExternalReceipts(expectedDocumentRevision int, state ActivationState) (int, error) {
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
	if document.Revision != expectedDocumentRevision {
		return document.Revision, StalePlanError{Precondition: fmt.Sprintf("capability-pack state revision changed from %d to %d before persistence; rerun activation to preview a fresh plan", expectedDocumentRevision, document.Revision)}
	}
	durable := receiptDocumentFromActivation(document)
	existing := map[string]string{}
	for _, effect := range document.External {
		if effect.Receipt != nil {
			existing[effect.ID] = digestJSON(effect)
		}
	}
	changed := false
	for _, effect := range state.External {
		if effect.Receipt == nil || existing[effect.ID] == digestJSON(effect) {
			continue
		}
		if err := canonicalizeExternalReceipt(&effect, effect.Receipt.Surface); err != nil {
			return document.Revision, err
		}
		replaced := false
		for i := range durable.ExternalEffects {
			if durable.ExternalEffects[i].ID == effect.ID {
				durable.ExternalEffects[i] = cloneExternalEffects([]ExternalEffect{effect})[0]
				replaced = true
				break
			}
		}
		if !replaced {
			durable.ExternalEffects = append(durable.ExternalEffects, cloneExternalEffects([]ExternalEffect{effect})[0])
		}
		existing[effect.ID] = digestJSON(effect)
		changed = true
	}
	if !changed {
		return document.Revision, nil
	}
	sort.Slice(durable.ExternalEffects, func(i, j int) bool { return durable.ExternalEffects[i].ID < durable.ExternalEffects[j].ID })
	durable.Revision++
	data, err := json.MarshalIndent(durable, "", "  ")
	if err != nil {
		return document.Revision, fmt.Errorf("encode capability-pack state: %w", err)
	}
	if err := atomicWriteState(s.path, append(data, '\n')); err != nil {
		return document.Revision, err
	}
	return durable.Revision, nil
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
	document.External = verifiedExternalEffects(state.External)
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
	state.External = cloneExternalEffects(document.External)
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
	recordedEffects := map[string]bool{}
	for _, state := range document.Activations {
		for _, intent := range activeIntents(state) {
			if !intent.Active {
				continue
			}
			receipt := installedPackReceipt{
				Pack: installedPackIdentity{ID: intent.PackID, Version: intent.Version}, Surface: intent.Surface,
				Selection: cloneSelection(intent.Selection), Aliases: cloneAliases(intent.Aliases),
				Resources: append([]ResourceIdentity(nil), intent.Resources...),
			}
			for _, owner := range document.Ownership {
				if !ownershipBelongsToReceipt(owner, intent.PackID, intent.Surface) {
					continue
				}
				receipt.Projections = append(receipt.Projections, installedProjection{
					ID: owner.ProjectionID, PhysicalID: owner.ID, Target: owner.Target, Digest: owner.Fingerprint,
					Contributors: append([]string(nil), owner.Contributors...), AdapterProvenance: owner.AdapterProvenance,
					Authorities: append([]ProjectionAuthority(nil), owner.Authorities...),
				})
			}
			for _, effect := range document.External {
				if effect.Receipt != nil && effect.Receipt.Surface == intent.Surface && contributorsContainPack(effect.Receipt.Contributors, intent.PackID) {
					receipt.ExternalEffects = append(receipt.ExternalEffects, cloneExternalEffects([]ExternalEffect{effect})[0])
					recordedEffects[effect.ID] = true
				}
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
	for _, effect := range document.External {
		if effect.Receipt != nil && !recordedEffects[effect.ID] {
			receipts.ExternalEffects = append(receipts.ExternalEffects, cloneExternalEffects([]ExternalEffect{effect})[0])
		}
	}
	sort.Slice(receipts.ExternalEffects, func(i, j int) bool { return receipts.ExternalEffects[i].ID < receipts.ExternalEffects[j].ID })
	return receipts
}

func activationDocumentFromReceipts(receipts installedReceiptDocument) (activationDocument, error) {
	document := activationDocument{SchemaVersion: 1, Revision: receipts.Revision, Activations: []ActivationState{}, Ownership: []ProjectionOwnership{}}
	bySurface := map[Surface]*ActivationState{}
	owners := map[string]ProjectionOwnership{}
	external := map[string]ExternalEffect{}
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
		intent := ActivationIntent{PackID: receipt.Pack.ID, Version: receipt.Pack.Version, Surface: receipt.Surface, Active: true, Revision: receipts.Revision, Aliases: cloneAliases(receipt.Aliases), Selection: cloneSelection(receipt.Selection), Resources: append([]ResourceIdentity(nil), receipt.Resources...), Explicit: &explicit}
		state.Intents = append(state.Intents, intent)
		state.Intent = intent
		for _, projection := range receipt.Projections {
			owner := ProjectionOwnership{ID: projection.PhysicalID, ProjectionID: projection.ID, Target: projection.Target, Fingerprint: projection.Digest, Contributors: append([]string(nil), projection.Contributors...), AdapterProvenance: projection.AdapterProvenance, Authorities: append([]ProjectionAuthority(nil), projection.Authorities...)}
			if owner.ID == "" {
				owner.ID = projectionOwnershipKey(projection)
			}
			if existing, ok := owners[owner.ID]; ok {
				existing.Contributors = sortedUnique(append(existing.Contributors, owner.Contributors...))
				owners[owner.ID] = existing
			} else {
				owners[owner.ID] = owner
			}
		}
		for _, effect := range receipt.ExternalEffects {
			external[effect.ID] = effect
		}
	}
	for _, state := range bySurface {
		sort.Slice(state.Intents, func(i, j int) bool { return state.Intents[i].PackID < state.Intents[j].PackID })
		document.Activations = append(document.Activations, *state)
	}
	for _, effect := range receipts.ExternalEffects {
		external[effect.ID] = effect
	}
	for _, effect := range external {
		document.External = append(document.External, effect)
	}
	for _, owner := range owners {
		document.Ownership = append(document.Ownership, owner)
	}
	if err := canonicalizeActivationDocument(&document); err != nil {
		return activationDocument{}, err
	}
	return document, nil
}

func projectionOwnershipKey(projection installedProjection) string {
	if projection.Target != "" {
		return "path:" + filepath.Clean(projection.Target)
	}
	return projection.ID
}

func ownershipBelongsToReceipt(owner ProjectionOwnership, packID string, surface Surface) bool {
	for _, contributor := range owner.Contributors {
		if contributorBelongsToPack(contributor, packID) && strings.HasPrefix(contributor, "surface:"+string(surface)+":") {
			return true
		}
	}
	return false
}

func contributorsContainPack(contributors []string, packID string) bool {
	for _, contributor := range contributors {
		if contributorBelongsToPack(contributor, packID) {
			return true
		}
	}
	return false
}

func verifiedExternalEffects(effects []ExternalEffect) []ExternalEffect {
	verified := make([]ExternalEffect, 0, len(effects))
	for _, effect := range effects {
		if effect.Receipt != nil {
			verified = append(verified, cloneExternalEffects([]ExternalEffect{effect})[0])
		}
	}
	return verified
}

func canonicalizeActivationDocument(document *activationDocument) error {
	if document.SchemaVersion < 5 {
		document.SchemaVersion = 5
	}
	for i := range document.Activations {
		if err := canonicalizeActivationState(&document.Activations[i]); err != nil {
			return err
		}
	}
	for i := range document.Ownership {
		document.Ownership[i].Contributors = sortedUnique(document.Ownership[i].Contributors)
		sort.Slice(document.Ownership[i].Authorities, func(a, b int) bool {
			return document.Ownership[i].Authorities[a].Surface < document.Ownership[i].Authorities[b].Surface
		})
	}
	sort.Slice(document.Ownership, func(i, j int) bool { return document.Ownership[i].ID < document.Ownership[j].ID })
	for i := range document.External {
		if document.External[i].Receipt == nil {
			return fmt.Errorf("external effect %q is missing its verified receipt", document.External[i].ID)
		}
		if err := canonicalizeExternalReceipt(&document.External[i], document.External[i].Receipt.Surface); err != nil {
			return err
		}
	}
	sort.Slice(document.External, func(i, j int) bool { return document.External[i].ID < document.External[j].ID })
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
	seenEffects := map[string]bool{}
	for i := range state.External {
		effect := &state.External[i]
		if seenEffects[effect.ID] {
			return fmt.Errorf("duplicate external effect %q", effect.ID)
		}
		seenEffects[effect.ID] = true
		if effect.Receipt == nil {
			continue
		}
		if err := canonicalizeExternalReceipt(effect, effect.Receipt.Surface); err != nil {
			return err
		}
	}
	sort.Slice(state.External, func(i, j int) bool { return state.External[i].ID < state.External[j].ID })
	return nil
}

func canonicalizeExternalReceipt(effect *ExternalEffect, surface Surface) error {
	receipt := effect.Receipt
	if receipt.SchemaVersion != 1 || receipt.Reversal.SchemaVersion != 1 {
		return fmt.Errorf("external effect %q has unsupported receipt schema", effect.ID)
	}
	if receipt.EffectID != effect.ID || receipt.EffectFingerprint == "" || receipt.EffectFingerprint != effect.Fingerprint {
		return fmt.Errorf("external effect %q receipt identity does not match its sealed effect", effect.ID)
	}
	if receipt.Surface == "" || surface != "" && receipt.Surface != surface {
		return fmt.Errorf("external effect %q receipt targets surface %q instead of %q", effect.ID, receipt.Surface, surface)
	}
	if receipt.Reversal.Consent != ConsentDestructiveCleanup || len(receipt.Reversal.AuthorityLimits) == 0 {
		return fmt.Errorf("external effect %q receipt has an invalid reversal contract", effect.ID)
	}
	receipt.Contributors = sortedUnique(receipt.Contributors)
	if len(receipt.Contributors) == 0 || len(receipt.Contributions) == 0 {
		return fmt.Errorf("external effect %q receipt has no sealed contributors or contributions", effect.ID)
	}
	seen := map[string]bool{}
	for _, contribution := range receipt.Contributions {
		if contribution.ID == "" || contribution.ObservedFingerprint == "" || contribution.AdapterProvenance == "" || seen[contribution.ID] {
			return fmt.Errorf("external effect %q receipt has an invalid or duplicate contribution", effect.ID)
		}
		seen[contribution.ID] = true
	}
	sort.Slice(receipt.Contributions, func(i, j int) bool { return receipt.Contributions[i].ID < receipt.Contributions[j].ID })
	receipt.Reversal.AuthorityLimits = sortedUnique(receipt.Reversal.AuthorityLimits)
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
