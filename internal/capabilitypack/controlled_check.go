package capabilitypack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const controlledCheckSchemaVersion = 1

type ControlledCheckScope string

const (
	ControlledCheckGlobal  ControlledCheckScope = "global"
	ControlledCheckProject ControlledCheckScope = "project"
)

type ControlledCheckState string

const (
	ControlledCheckUnknown ControlledCheckState = "unknown"
	ControlledCheckCurrent ControlledCheckState = "current"
	ControlledCheckStale   ControlledCheckState = "stale"
)

// ControlledCheckDescriptor comes from a surface adapter. Its values are
// facts, not probes: Packy never runs a controlled check on the user's behalf.
type ControlledCheckDescriptor struct {
	AdapterVersion string   `json:"adapter_version"`
	HostVersion    string   `json:"host_version"`
	Instructions   []string `json:"instructions"`
}

type ControlledCheckIdentity struct {
	Pack               string               `json:"pack"`
	PackVersion        string               `json:"pack_version"`
	Scope              ControlledCheckScope `json:"scope"`
	ProjectDigest      string               `json:"project_digest,omitempty"`
	Surface            Surface              `json:"surface"`
	Resources          []ResourceIdentity   `json:"resources"`
	ProjectionRevision string               `json:"projection_revision"`
	AdapterVersion     string               `json:"adapter_version"`
	HostVersion        string               `json:"host_version"`
}

type ControlledCheckStatus struct {
	State            ControlledCheckState `json:"state"`
	Result           ReadinessValue       `json:"result,omitempty"`
	ObservedAt       string               `json:"observed_at,omitempty"`
	ValidityIdentity string               `json:"validity_identity"`
}

// ControlledCheckPreview is the exact human operation that may be recorded.
// The private request prevents callers from synthesizing a preview at Record
// time; Record recomputes it and rejects an identity that has changed.
type ControlledCheckPreview struct {
	SchemaVersion      int                   `json:"schema_version"`
	Report             string                `json:"report"`
	Pack               string                `json:"pack"`
	PackVersion        string                `json:"pack_version"`
	Surface            Surface               `json:"surface"`
	Scope              ControlledCheckScope  `json:"scope"`
	ProjectDigest      string                `json:"project_digest,omitempty"`
	Resources          []ResourceIdentity    `json:"resources"`
	ProjectionRevision string                `json:"projection_revision"`
	AdapterVersion     string                `json:"adapter_version"`
	HostVersion        string                `json:"host_version"`
	Instructions       []string              `json:"instructions"`
	ValidityIdentity   string                `json:"validity_identity"`
	CurrentEvidence    ControlledCheckStatus `json:"current_evidence"`
	request            ControlledCheckRequest
}

type ControlledCheckRequest struct {
	PackID      string
	Surface     Surface
	ProjectRoot string
	PackyHome   string
	Selection   ResourceSelection
	Adapter     SurfaceAdapter
}

type controlledCheckEvidence struct {
	Identity         ControlledCheckIdentity `json:"identity"`
	ValidityIdentity string                  `json:"validity_identity"`
	Result           ReadinessValue          `json:"result"`
	ObservedAt       string                  `json:"observed_at"`
}

type controlledCheckDocument struct {
	SchemaVersion int                       `json:"schema_version"`
	Entries       []controlledCheckEvidence `json:"entries"`
}

// FileControlledCheckStore owns only personal, workstation-local evidence.
// It intentionally has no relationship to installed receipts or project
// artifacts, making copied project state non-portable by construction.
type FileControlledCheckStore struct {
	path string
	mu   sync.Mutex
}

type ControlledCheckEvidenceStore interface {
	Status(context.Context, ControlledCheckIdentity) (ControlledCheckStatus, error)
	Record(context.Context, ControlledCheckIdentity, ReadinessValue, time.Time) (ControlledCheckStatus, error)
}

func NewFileControlledCheckStore(packyHome string) *FileControlledCheckStore {
	return &FileControlledCheckStore{path: filepath.Join(packyHome, "controlled-checks.json")}
}

func (f Facade) PreviewControlledCheck(ctx context.Context, request ControlledCheckRequest) (ControlledCheckPreview, error) {
	identity, instructions, err := f.controlledCheckIdentity(ctx, request)
	if err != nil {
		return ControlledCheckPreview{}, err
	}
	store := f.controlledCheckStore(request.PackyHome)
	current, err := store.Status(ctx, identity)
	if err != nil {
		return ControlledCheckPreview{}, err
	}
	return controlledCheckPreview(identity, instructions, current, request), nil
}

// RecordControlledCheck stores only an explicit positive or negative result.
// It recomputes the preview so a changed Pack, selection, projection, adapter,
// or host cannot receive evidence intended for the earlier identity.
func (f Facade) RecordControlledCheck(ctx context.Context, preview ControlledCheckPreview, result ReadinessValue) (ControlledCheckStatus, error) {
	if result != ReadinessTrue && result != ReadinessFalse {
		return ControlledCheckStatus{}, errors.New("controlled check result must be true or false")
	}
	if preview.request.PackyHome == "" || preview.ValidityIdentity == "" {
		return ControlledCheckStatus{}, errors.New("controlled check record requires a preview returned by Packy")
	}
	fresh, err := f.PreviewControlledCheck(ctx, preview.request)
	if err != nil {
		return ControlledCheckStatus{}, err
	}
	if fresh.ValidityIdentity != preview.ValidityIdentity {
		return ControlledCheckStatus{}, fmt.Errorf("controlled check preview is stale; rerun the check preview before recording evidence")
	}
	return f.controlledCheckStore(preview.request.PackyHome).Record(ctx, fresh.identity(), result, time.Now().UTC().Truncate(time.Second))
}

func (f Facade) controlledCheckStore(packyHome string) ControlledCheckEvidenceStore {
	if f.controlledChecks != nil {
		return f.controlledChecks
	}
	if packyHome == "" {
		return nil
	}
	return NewFileControlledCheckStore(packyHome)
}

func (p ControlledCheckPreview) identity() ControlledCheckIdentity {
	return ControlledCheckIdentity{Pack: p.Pack, PackVersion: p.PackVersion, Scope: p.Scope, ProjectDigest: p.ProjectDigest, Surface: p.Surface, Resources: append([]ResourceIdentity(nil), p.Resources...), ProjectionRevision: p.ProjectionRevision, AdapterVersion: p.AdapterVersion, HostVersion: p.HostVersion}
}

func controlledCheckPreview(identity ControlledCheckIdentity, instructions []string, current ControlledCheckStatus, request ControlledCheckRequest) ControlledCheckPreview {
	return ControlledCheckPreview{SchemaVersion: controlledCheckSchemaVersion, Report: "controlled-check-preview", Pack: identity.Pack, PackVersion: identity.PackVersion, Surface: identity.Surface, Scope: identity.Scope, ProjectDigest: identity.ProjectDigest, Resources: append([]ResourceIdentity(nil), identity.Resources...), ProjectionRevision: identity.ProjectionRevision, AdapterVersion: identity.AdapterVersion, HostVersion: identity.HostVersion, Instructions: append([]string(nil), instructions...), ValidityIdentity: controlledCheckValidity(identity), CurrentEvidence: current, request: request}
}

func (f Facade) controlledCheckIdentity(ctx context.Context, request ControlledCheckRequest) (ControlledCheckIdentity, []string, error) {
	if request.PackID == "" || request.Surface == "" || request.PackyHome == "" {
		return ControlledCheckIdentity{}, nil, errors.New("controlled check requires Pack, surface, and Packy Home")
	}
	if request.ProjectRoot != "" {
		return f.projectControlledCheckIdentity(ctx, request)
	}
	if f.activation == nil || f.activation.store == nil {
		return ControlledCheckIdentity{}, nil, errors.New("controlled check requires activation state")
	}
	state, err := f.activation.store.LoadSnapshot(ctx, request.Surface)
	if err != nil {
		return ControlledCheckIdentity{}, nil, err
	}
	pack, err := f.catalog.catalogMetadata(request.PackID)
	if err != nil {
		return ControlledCheckIdentity{}, nil, err
	}
	selection := request.Selection
	if intent, ok := intentForPack(state, request.PackID, request.Surface); ok {
		selection = intent.Selection
		pack, err = f.catalog.resolveIntentPack(ctx, intent.PackID, intent.Version)
		if err != nil {
			return ControlledCheckIdentity{}, nil, err
		}
	}
	selection, err = canonicalSelection(selection)
	if err != nil {
		return ControlledCheckIdentity{}, nil, err
	}
	adapter := request.Adapter
	if adapter == nil {
		adapter = f.activation.adapters[request.Surface]
	}
	if adapter == nil {
		return ControlledCheckIdentity{}, nil, fmt.Errorf("no activation adapter configured for CLI surface %q", request.Surface)
	}
	selectedPack, err := selectPackResources(pack, selection)
	if err != nil {
		return ControlledCheckIdentity{}, nil, err
	}
	relevantPack, err := f.statusEvidencePack(selectedPack, request.Surface)
	if err != nil {
		return ControlledCheckIdentity{}, nil, err
	}
	observation, err := inspectSurface(ctx, adapter, SurfaceTransition{Desired: relevantPack, CurrentOwnership: state.Ownership})
	if err != nil {
		return ControlledCheckIdentity{}, nil, err
	}
	resources := controlledCheckResources(ResourceGraphFor(pack, selection, false))
	descriptor := normalizedControlledCheckDescriptor(request.Surface, observation.ControlledCheck)
	return controlledCheckIdentityFor(pack, request.Surface, ControlledCheckGlobal, "", resources, observation, descriptor), descriptor.Instructions, nil
}

func (f Facade) projectControlledCheckIdentity(ctx context.Context, request ControlledCheckRequest) (ControlledCheckIdentity, []string, error) {
	installation, err := LoadProjectInstallation(request.ProjectRoot)
	if err != nil {
		return ControlledCheckIdentity{}, nil, err
	}
	projectPack, found := findProjectManifestPack(installation.Manifest.Packs, request.PackID)
	if !found || !containsSurface(projectPack.Surfaces, request.Surface) {
		return ControlledCheckIdentity{}, nil, fmt.Errorf("pack %q on %s is not declared by this project installation", request.PackID, request.Surface)
	}
	version := projectPack.Version
	for _, intent := range projectSurfaceIntents(projectPack) {
		if intent.Surface == request.Surface {
			version = intent.Version
		}
	}
	pack, err := f.catalog.resolveIntentPack(ctx, request.PackID, version)
	if err != nil {
		return ControlledCheckIdentity{}, nil, err
	}
	adapter := request.Adapter
	if adapter == nil {
		return ControlledCheckIdentity{}, nil, fmt.Errorf("project controlled check does not support CLI surface %q", request.Surface)
	}
	scoped := projectInstallationForPack(installation, request.PackID)
	observation, err := inspectSurface(ctx, adapter, SurfaceTransition{ProjectRoot: request.ProjectRoot, ProjectInstallation: &scoped, ProjectGoal: ProjectionPresent})
	if err != nil {
		return ControlledCheckIdentity{}, nil, err
	}
	digest, err := projectActivationRootDigest(request.ProjectRoot)
	if err != nil {
		return ControlledCheckIdentity{}, nil, err
	}
	resources := controlledCheckResources(projectLockForPack(installation.Lock, request.PackID).ResourceGraph)
	descriptor := normalizedControlledCheckDescriptor(request.Surface, observation.ControlledCheck)
	return controlledCheckIdentityFor(pack, request.Surface, ControlledCheckProject, digest, resources, observation, descriptor), descriptor.Instructions, nil
}

func containsSurface(values []Surface, target Surface) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func controlledCheckIdentityFor(pack Pack, surface Surface, scope ControlledCheckScope, projectDigest string, resources []ResourceIdentity, observation SurfaceInspection, descriptor ControlledCheckDescriptor) ControlledCheckIdentity {
	revision := observation.Revision
	if revision == "" {
		revision = observationDigest(observation)
	}
	return ControlledCheckIdentity{Pack: pack.ID, PackVersion: pack.Version, Scope: scope, ProjectDigest: projectDigest, Surface: surface, Resources: resources, ProjectionRevision: revision, AdapterVersion: descriptor.AdapterVersion, HostVersion: descriptor.HostVersion}
}

func normalizedControlledCheckDescriptor(surface Surface, value ControlledCheckDescriptor) ControlledCheckDescriptor {
	if value.AdapterVersion == "" {
		value.AdapterVersion = string(surface) + "-adapter/v1"
	}
	if value.HostVersion == "" {
		value.HostVersion = "unobservable"
	}
	if len(value.Instructions) == 0 {
		value.Instructions = []string{fmt.Sprintf("Verify the selected Pack behavior in %s, then record whether it succeeded.", surface)}
	}
	value.Instructions = append([]string(nil), value.Instructions...)
	for i := range value.Instructions {
		value.Instructions[i] = strings.TrimSpace(value.Instructions[i])
	}
	return value
}

func controlledCheckResources(graph ResourceGraph) []ResourceIdentity {
	resources := make([]ResourceIdentity, 0, len(graph.Resources))
	for _, fact := range graph.Resources {
		if fact.Role != ResourceRoleUnselected {
			resources = append(resources, fact.Resource)
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].String() < resources[j].String() })
	return resources
}

func controlledCheckValidity(identity ControlledCheckIdentity) string { return digestJSON(identity) }

func (s *FileControlledCheckStore) Status(ctx context.Context, identity ControlledCheckIdentity) (ControlledCheckStatus, error) {
	document, err := s.Load(ctx)
	if err != nil {
		return ControlledCheckStatus{}, err
	}
	validity := controlledCheckValidity(identity)
	var stale *controlledCheckEvidence
	for i := range document.Entries {
		entry := document.Entries[i]
		if entry.ValidityIdentity == validity {
			return ControlledCheckStatus{State: ControlledCheckCurrent, Result: entry.Result, ObservedAt: entry.ObservedAt, ValidityIdentity: validity}, nil
		}
		if controlledCheckSlot(entry.Identity) == controlledCheckSlot(identity) && (stale == nil || entry.ObservedAt > stale.ObservedAt) {
			copy := entry
			stale = &copy
		}
	}
	if stale != nil {
		return ControlledCheckStatus{State: ControlledCheckStale, Result: stale.Result, ObservedAt: stale.ObservedAt, ValidityIdentity: stale.ValidityIdentity}, nil
	}
	return ControlledCheckStatus{State: ControlledCheckUnknown, ValidityIdentity: validity}, nil
}

func controlledCheckSlot(identity ControlledCheckIdentity) string {
	return string(identity.Scope) + "\x00" + identity.ProjectDigest + "\x00" + identity.Pack + "\x00" + string(identity.Surface)
}

func (s *FileControlledCheckStore) Record(ctx context.Context, identity ControlledCheckIdentity, result ReadinessValue, observedAt time.Time) (ControlledCheckStatus, error) {
	evidence := controlledCheckEvidence{Identity: identity, ValidityIdentity: controlledCheckValidity(identity), Result: result, ObservedAt: observedAt.UTC().Truncate(time.Second).Format(time.RFC3339)}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateControlledCheckEvidence(evidence); err != nil {
		return ControlledCheckStatus{}, err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return ControlledCheckStatus{}, fmt.Errorf("create controlled check directory: %w", err)
	}
	lock, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return ControlledCheckStatus{}, fmt.Errorf("open controlled check lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return ControlledCheckStatus{}, fmt.Errorf("lock controlled checks: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	document, err := s.load()
	if err != nil {
		return ControlledCheckStatus{}, err
	}
	replaced := false
	for i := range document.Entries {
		if controlledCheckSlot(document.Entries[i].Identity) == controlledCheckSlot(evidence.Identity) {
			document.Entries[i] = evidence
			replaced = true
			break
		}
	}
	if !replaced {
		document.Entries = append(document.Entries, evidence)
	}
	if err := canonicalizeControlledCheckDocument(&document); err != nil {
		return ControlledCheckStatus{}, err
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return ControlledCheckStatus{}, fmt.Errorf("encode controlled checks: %w", err)
	}
	if err := atomicWriteState(s.path, append(data, '\n')); err != nil {
		return ControlledCheckStatus{}, err
	}
	return ControlledCheckStatus{State: ControlledCheckCurrent, Result: evidence.Result, ObservedAt: evidence.ObservedAt, ValidityIdentity: evidence.ValidityIdentity}, nil
}

func (s *FileControlledCheckStore) Load(context.Context) (controlledCheckDocument, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *FileControlledCheckStore) load() (controlledCheckDocument, error) {
	info, err := os.Lstat(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return controlledCheckDocument{SchemaVersion: controlledCheckSchemaVersion, Entries: []controlledCheckEvidence{}}, nil
	}
	if err != nil {
		return controlledCheckDocument{}, fmt.Errorf("inspect controlled checks: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return controlledCheckDocument{}, errors.New("controlled checks must be a private regular file")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return controlledCheckDocument{}, fmt.Errorf("read controlled checks: %w", err)
	}
	var document controlledCheckDocument
	if err := strictDecode(data, &document); err != nil {
		return controlledCheckDocument{}, fmt.Errorf("decode controlled checks: %w", err)
	}
	if err := canonicalizeControlledCheckDocument(&document); err != nil {
		return controlledCheckDocument{}, fmt.Errorf("controlled checks: %w", err)
	}
	return document, nil
}

func canonicalizeControlledCheckDocument(document *controlledCheckDocument) error {
	if document.SchemaVersion != controlledCheckSchemaVersion {
		return fmt.Errorf("unsupported schema_version %d", document.SchemaVersion)
	}
	if document.Entries == nil {
		document.Entries = []controlledCheckEvidence{}
	}
	seen := map[string]bool{}
	for i := range document.Entries {
		if err := validateControlledCheckEvidence(document.Entries[i]); err != nil {
			return err
		}
		if seen[document.Entries[i].ValidityIdentity] {
			return fmt.Errorf("duplicate validity identity")
		}
		seen[document.Entries[i].ValidityIdentity] = true
	}
	sort.Slice(document.Entries, func(i, j int) bool {
		return document.Entries[i].ValidityIdentity < document.Entries[j].ValidityIdentity
	})
	return nil
}

func validateControlledCheckEvidence(value controlledCheckEvidence) error {
	i := value.Identity
	if i.Pack == "" || i.PackVersion == "" || i.Surface == "" || i.ProjectionRevision == "" || i.AdapterVersion == "" || i.HostVersion == "" {
		return errors.New("identity is incomplete")
	}
	if i.Scope != ControlledCheckGlobal && i.Scope != ControlledCheckProject {
		return errors.New("identity scope is invalid")
	}
	if (i.Scope == ControlledCheckProject) != (i.ProjectDigest != "") {
		return errors.New("identity project scope is invalid")
	}
	if value.Result != ReadinessTrue && value.Result != ReadinessFalse {
		return errors.New("result must be true or false")
	}
	if _, err := time.Parse(time.RFC3339, value.ObservedAt); err != nil {
		return errors.New("observed_at is invalid")
	}
	if value.ValidityIdentity != controlledCheckValidity(i) {
		return errors.New("validity identity does not match identity")
	}
	if !sort.SliceIsSorted(i.Resources, func(a, b int) bool { return i.Resources[a].String() < i.Resources[b].String() }) {
		return errors.New("resources are not canonical")
	}
	for n, resource := range i.Resources {
		if resource.Kind == "" || resource.ID == "" || n > 0 && i.Resources[n-1] == resource {
			return errors.New("resources are invalid")
		}
	}
	return nil
}
