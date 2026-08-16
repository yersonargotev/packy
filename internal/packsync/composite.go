package packsync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/yersonargotev/packy/internal/bundletransaction"
)

// CompositeLegalAdmission is the durable, digest-bound redistribution
// authority for one member of a Composite Pack Source Bundle.
type CompositeLegalAdmission struct {
	EvidenceReference string `json:"evidence_reference"`
	EvidenceSHA256    string `json:"evidence_sha256"`
	Disposition       string `json:"disposition"`
}

type CompositeRegistrationMember struct {
	Registration   SourceConfig            `json:"registration"`
	LegalAdmission CompositeLegalAdmission `json:"legal_admission"`
}

type CompositeCheckRequest struct {
	RepositoryRoot   string
	AcquisitionDir   string
	PackID           string
	ProposedVersion  string
	ProposedManifest json.RawMessage
	Members          []CompositeRegistrationMember
}

type CompositeMemberPlan struct {
	SourceID         string                  `json:"source_id"`
	Registration     SourceConfig            `json:"registration"`
	Candidate        Candidate               `json:"candidate"`
	LegalAdmission   CompositeLegalAdmission `json:"legal_admission"`
	SourceLockSHA256 string                  `json:"source_lock_sha256"`
	ProposedLock     Lock                    `json:"proposed_lock"`
}

type CompositePlan struct {
	SchemaVersion            int                   `json:"schema_version"`
	PlanID                   string                `json:"plan_id"`
	Status                   string                `json:"status"`
	PackID                   string                `json:"pack_id"`
	ProposedVersion          string                `json:"proposed_version"`
	ProposedManifest         json.RawMessage       `json:"proposed_manifest"`
	ProposedManifestSHA256   string                `json:"proposed_manifest_sha256"`
	SourceIDs                []string              `json:"source_ids"`
	RegistrationBundleSHA256 string                `json:"registration_bundle_sha256"`
	Members                  []CompositeMemberPlan `json:"members"`
	Preconditions            Preconditions         `json:"preconditions"`
	ResultingConfigSHA256    string                `json:"resulting_config_sha256"`
	LockSetSHA256            string                `json:"lock_set_sha256"`
	ResultBundleSHA256       string                `json:"result_bundle_sha256"`
	Blockers                 []string              `json:"blockers"`
}

func (plan CompositePlan) VerifySeal() bool {
	want, err := sealCompositePlan(plan)
	return err == nil && want == plan.PlanID
}

type CompositeApplyRequest struct {
	CompositeCheckRequest
	Plan                   CompositePlan
	ClassificationEvidence CompositeClassificationEvidence
}

// CompositeClassificationEvidence is one Pack-level decision bound to the
// complete composite plan. It intentionally has no member-level authority.
type CompositeClassificationEvidence struct {
	SchemaVersion int                    `json:"schema_version"`
	PlanID        string                 `json:"plan_id"`
	PackID        string                 `json:"pack_id"`
	Evidence      ClassificationEvidence `json:"evidence"`
}

func ValidateCompositeClassificationEvidence(plan CompositePlan, set CompositeClassificationEvidence) error {
	evidence := set.Evidence
	if !plan.VerifySeal() || set.SchemaVersion != 1 || set.PlanID != plan.PlanID || set.PackID != plan.PackID ||
		evidence.PackID != plan.PackID || evidence.CurrentVersion != "0.0.0" ||
		evidence.ProposedVersion != plan.ProposedVersion || evidence.MechanicalFloor != LevelMajor {
		return errors.New("composite classification evidence is stale or does not cover the complete Pack plan")
	}
	impact := PackImpact{PackID: plan.PackID, CurrentVersion: "0.0.0", MechanicalFloor: LevelMajor, SemanticEvidenceRequired: true}
	if err := validatePackClassification(impact, evidence); err != nil {
		return fmt.Errorf("composite Pack classification: %w", err)
	}
	return nil
}

// CanonicalRegistrationBundle seals the complete ordered registration intent.
func CanonicalRegistrationBundle(members []CompositeRegistrationMember) ([]CompositeRegistrationMember, string, error) {
	if len(members) < 2 {
		return nil, "", errors.New("composite registration requires at least two members")
	}
	ordered := append([]CompositeRegistrationMember(nil), members...)
	for i := range ordered {
		normalized, _, err := canonicalRegistration(ordered[i].Registration)
		if err != nil {
			return nil, "", err
		}
		ordered[i].Registration = normalized
		if !fullDigest(ordered[i].LegalAdmission.EvidenceSHA256) ||
			ordered[i].LegalAdmission.EvidenceReference == "" ||
			ordered[i].LegalAdmission.Disposition != RedistributableDisposition {
			return nil, "", fmt.Errorf("source %q has incomplete legal admission evidence", normalized.ID)
		}
		if normalized.Selector.Mode != SelectorCommit {
			return nil, "", fmt.Errorf("source %q requires an exact commit selector", normalized.ID)
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Registration.ID < ordered[j].Registration.ID })
	for i := 1; i < len(ordered); i++ {
		if ordered[i-1].Registration.ID == ordered[i].Registration.ID {
			return nil, "", fmt.Errorf("duplicate source id %q", ordered[i].Registration.ID)
		}
	}
	data, err := json.Marshal(ordered)
	if err != nil {
		return nil, "", err
	}
	return ordered, hashBytes(data), nil
}

func (engine Engine) CheckComposite(ctx context.Context, request CompositeCheckRequest) (CompositePlan, error) {
	if engine.Source == nil || engine.Validate == nil || request.RepositoryRoot == "" || request.AcquisitionDir == "" || request.PackID == "" || request.ProposedVersion == "" {
		return CompositePlan{}, errors.New("composite Check requires source acquisition, Packy validation, repository root, acquisition directory, Pack, and proposed version")
	}
	manifest, manifestBytes, err := validateCompositeManifest(request.PackID, request.ProposedVersion, request.ProposedManifest)
	if err != nil {
		return CompositePlan{}, err
	}
	if err := requireEmptyDirectory(request.AcquisitionDir); err != nil {
		return CompositePlan{}, fmt.Errorf("acquisition directory: %w", err)
	}
	members, registrationDigest, err := CanonicalRegistrationBundle(request.Members)
	if err != nil {
		return CompositePlan{}, err
	}
	for _, member := range members {
		for _, binding := range member.Registration.Resources {
			if binding.PackID != request.PackID {
				return CompositePlan{}, fmt.Errorf("source %q binds outside declared Pack %q", member.Registration.ID, request.PackID)
			}
		}
	}

	initial, err := readCompositeLocal(ctx, request.RepositoryRoot, members, request.PackID, false)
	if err != nil {
		return CompositePlan{}, err
	}
	candidates := make([]Candidate, len(members))
	for i, member := range members {
		candidates[i], err = engine.Source.ResolveCommit(ctx, member.Registration, member.Registration.Selector.Ref)
		if err != nil {
			return CompositePlan{}, fmt.Errorf("resolve member %s: %w", member.Registration.ID, err)
		}
	}
	roots := make([]string, len(members))
	err = engine.withCompositeSnapshots(ctx, members, candidates, request.AcquisitionDir, roots, 0, func() error {
		guard, err := bundletransaction.Acquire(ctx, request.RepositoryRoot)
		if err != nil {
			return err
		}
		defer guard.Release()
		fresh, err := readCompositeLocalUnlocked(request.RepositoryRoot, members, request.PackID, false)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(initial, fresh) {
			return errors.New("local bundle facts changed during composite Check; retry")
		}
		manifests, manifestsHash, err := loadManifests(request.RepositoryRoot)
		if err != nil {
			return err
		}
		manifests[request.PackID] = manifest
		plan := CompositePlan{SchemaVersion: 1, Status: "review-required", PackID: request.PackID, ProposedVersion: request.ProposedVersion, ProposedManifest: manifestBytes, ProposedManifestSHA256: hashBytes(manifestBytes), RegistrationBundleSHA256: registrationDigest}
		digests := make([]SourceLockDigest, 0, len(initial.lockSet.Digests)+len(members))
		for id, digest := range initial.lockSet.Digests {
			digests = append(digests, SourceLockDigest{SourceID: id, SHA256: digest})
		}
		for i, member := range members {
			if blockers := validateCandidate(member.Registration, candidates[i], member.Registration.Selector); len(blockers) != 0 {
				return fmt.Errorf("source %s candidate is invalid: %v", member.Registration.ID, blockers)
			}
			if err := validateCompositeLegalAdmission(request.RepositoryRoot, member, candidates[i]); err != nil {
				return fmt.Errorf("source %s legal admission: %w", member.Registration.ID, err)
			}
			bindings, blockers := deriveDestinations(member.Registration.Resources, manifests)
			if len(blockers) != 0 {
				return fmt.Errorf("source %s bindings are invalid: %v", member.Registration.ID, blockers)
			}
			single := Plan{Registration: &member.Registration, Candidate: candidates[i], Selector: member.Registration.Selector}
			if err := buildPlan(roots[i], request.RepositoryRoot, member.Registration, bindings, manifests, Lock{}, false, initial.existingPacks, buildPlanCheck, &single); err != nil {
				return err
			}
			if len(single.Blockers) != 0 {
				return fmt.Errorf("source %s result is blocked: %v", member.Registration.ID, single.Blockers)
			}
			_, digest, err := CanonicalSourceLock(single.ProposedLock)
			if err != nil {
				return err
			}
			plan.SourceIDs = append(plan.SourceIDs, member.Registration.ID)
			plan.Members = append(plan.Members, CompositeMemberPlan{SourceID: member.Registration.ID, Registration: member.Registration, Candidate: candidates[i], LegalAdmission: member.LegalAdmission, SourceLockSHA256: digest, ProposedLock: single.ProposedLock})
			digests = append(digests, SourceLockDigest{SourceID: member.Registration.ID, SHA256: digest})
		}
		resultConfig := Config{SchemaVersion: initial.config.SchemaVersion, Sources: append(append([]SourceConfig(nil), initial.config.Sources...), registrations(members)...)}
		encoded, err := canonicalConfigAfterValidation(resultConfig)
		if err != nil {
			return fmt.Errorf("invalid complete resulting configuration: %w", err)
		}
		plan.ResultingConfigSHA256 = hashBytes(encoded)
		plan.LockSetSHA256, err = LockSetSHA256(digests)
		if err != nil {
			return err
		}
		base, err := repositoryBase(request.RepositoryRoot)
		if err != nil {
			return err
		}
		bundleHash, err := treeHash(filepath.Join(request.RepositoryRoot, "bundle"))
		if err != nil {
			return err
		}
		plan.Preconditions = Preconditions{BaseCommit: base, ConfigSHA256: hashBytes(initial.configBytes), ManifestsSHA256: manifestsHash, BundleSHA256: bundleHash, LockSetSHA256: initial.lockSet.LockSetSHA256}
		disposable := filepath.Join(request.AcquisitionDir, "complete-result")
		if err := copyTreeExact(filepath.Join(request.RepositoryRoot, "bundle"), disposable); err != nil {
			return fmt.Errorf("stage disposable complete result: %w", err)
		}
		defer os.RemoveAll(disposable)
		if err := materializeCompositeResult(disposable, plan, roots, initial.config, manifest); err != nil {
			return err
		}
		if err := engine.Validate.ValidateBundle(ctx, request.RepositoryRoot, disposable); err != nil {
			return fmt.Errorf("validate disposable complete composite result: %w", err)
		}
		plan.ResultBundleSHA256, err = treeHash(disposable)
		if err != nil {
			return err
		}
		plan.PlanID, err = sealCompositePlan(plan)
		if err != nil {
			return err
		}
		initial.plan = plan
		return nil
	})
	if err != nil {
		return CompositePlan{}, fmt.Errorf("inspect composite snapshots: %w", err)
	}
	if err := requireEmptyDirectory(request.AcquisitionDir); err != nil {
		return CompositePlan{}, fmt.Errorf("acquisition did not clean caller-supplied directory: %w", err)
	}
	return initial.plan, nil
}

func validateCompositeLegalAdmission(repositoryRoot string, member CompositeRegistrationMember, candidate Candidate) error {
	reference := member.LegalAdmission.EvidenceReference
	if !safeSlashPath(reference) {
		return errors.New("durable evidence reference is unsafe")
	}
	root, err := filepath.EvalSymlinks(repositoryRoot)
	if err != nil {
		return err
	}
	target, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(reference)))
	if err != nil {
		return fmt.Errorf("resolve durable evidence: %w", err)
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || !safeSlashPath(filepath.ToSlash(relative)) {
		return errors.New("durable evidence resolves outside repository")
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("read durable evidence: %w", err)
	}
	var evidence legalAdmissionEvidence
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil || ensureEOF(decoder) != nil {
		return ErrLegalAdmissionShape
	}
	selectedRoots := make([]string, 0, len(member.Registration.Resources))
	for _, binding := range member.Registration.Resources {
		selectedRoots = append(selectedRoots, binding.UpstreamPath)
	}
	sort.Strings(selectedRoots)
	expected := LegalAdmissionExpected{
		EvidenceReference: reference,
		EvidenceSHA256:    member.LegalAdmission.EvidenceSHA256,
		EvidenceID:        evidence.EvidenceID,
		Candidate:         evidence.Candidate,
		Scope:             LegalAdmissionScope{SelectedRoots: selectedRoots, Exclusions: append([]string(nil), evidence.Scope.Exclusions...)},
	}
	expected.Candidate.Repository = member.Registration.Repository
	expected.Candidate.Commit = candidate.Commit
	admission, err := ValidateLegalAdmission(raw, expected)
	if err != nil {
		return err
	}
	if admission.Disposition != member.LegalAdmission.Disposition {
		return ErrLegalAdmissionDisposition
	}
	return nil
}

type compositeLocal struct {
	configBytes   []byte
	config        Config
	lockSet       sourceLockSet
	existingPacks map[string]bool
	plan          CompositePlan
}

func readCompositeLocal(ctx context.Context, root string, members []CompositeRegistrationMember, packID string, allowSourceLess bool) (compositeLocal, error) {
	guard, err := bundletransaction.Acquire(ctx, root)
	if err != nil {
		return compositeLocal{}, err
	}
	defer guard.Release()
	return readCompositeLocalUnlocked(root, members, packID, allowSourceLess)
}

func readCompositeLocalUnlocked(root string, members []CompositeRegistrationMember, packID string, allowSourceLess bool) (compositeLocal, error) {
	packPath := filepath.Join(root, "bundle", "packs", packID)
	if _, err := os.Lstat(packPath); err == nil {
		return compositeLocal{}, fmt.Errorf("target Pack %q already exists", packID)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return compositeLocal{}, fmt.Errorf("inspect target Pack path: %w", err)
	}
	bundle := filepath.Join(root, "bundle")
	data, err := os.ReadFile(filepath.Join(bundle, "sources.json"))
	config := Config{}
	lockSet := sourceLockSet{}
	sourceLess := false
	if errors.Is(err, fs.ErrNotExist) {
		if !allowSourceLess {
			return compositeLocal{}, err
		}
		if _, lockErr := os.Lstat(filepath.Join(bundle, "sources")); lockErr == nil {
			return compositeLocal{}, errors.New("source configuration is absent while source locks exist")
		} else if !errors.Is(lockErr, fs.ErrNotExist) {
			return compositeLocal{}, lockErr
		}
		config = Config{SchemaVersion: 1, Sources: []SourceConfig{}}
		sourceLess = true
		lockSet, err = loadSourceLockSetForTarget(bundle, config, "", true)
	} else if err == nil {
		config, err = LoadConfig(bytes.NewReader(data))
	}
	if err != nil {
		return compositeLocal{}, err
	}
	existingPacks := map[string]bool{}
	existingSources := map[string]bool{}
	for _, source := range config.Sources {
		existingSources[source.ID] = true
		for _, binding := range source.Resources {
			existingPacks[binding.PackID] = true
		}
	}
	if existingPacks[packID] {
		return compositeLocal{}, fmt.Errorf("target Pack %q already exists", packID)
	}
	for _, member := range members {
		if existingSources[member.Registration.ID] {
			return compositeLocal{}, fmt.Errorf("member source %q is already configured", member.Registration.ID)
		}
	}
	if !sourceLess {
		lockSet, err = loadSourceLockSet(bundle, config)
		if err != nil {
			return compositeLocal{}, err
		}
	}
	return compositeLocal{configBytes: data, config: config, lockSet: lockSet, existingPacks: existingPacks}, nil
}

func registrations(members []CompositeRegistrationMember) []SourceConfig {
	result := make([]SourceConfig, len(members))
	for i := range members {
		result[i] = members[i].Registration
	}
	return result
}

func canonicalConfigAfterValidation(config Config) ([]byte, error) {
	raw, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	checked, err := LoadConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return canonicalConfig(checked)
}

func validateCompositeManifest(packID, version string, raw json.RawMessage) (packManifest, []byte, error) {
	if !canonicalSourceIDPattern.MatchString(packID) {
		return packManifest{}, nil, fmt.Errorf("unsafe target Pack id %q", packID)
	}
	canonical, err := CanonicalCompositePackManifest(raw)
	if err != nil {
		return packManifest{}, nil, errors.New("proposed Pack manifest is malformed")
	}
	var manifest packManifest
	if err := json.Unmarshal(canonical, &manifest); err != nil {
		return packManifest{}, nil, errors.New("proposed Pack manifest is malformed")
	}
	if manifest.SchemaVersion < 1 || manifest.SchemaVersion > 4 || manifest.ID != packID || manifest.Version != version || len(manifest.Resources) == 0 {
		return packManifest{}, nil, errors.New("proposed Pack manifest identity or version contradicts the composite request")
	}
	seen := map[string]bool{}
	for _, resource := range manifest.Resources {
		key := resource.Kind + "\x00" + resource.ID
		if resource.Kind == "" || resource.ID == "" || !safeSlashPath(resource.Source) || seen[key] {
			return packManifest{}, nil, errors.New("proposed Pack manifest resources are incomplete, unsafe, or duplicated")
		}
		seen[key] = true
	}
	return manifest, canonical, nil
}

// CanonicalCompositePackManifest returns the exact manifest representation
// sealed by composite Check and workflow artifacts. Unknown schema-specific
// fields are retained for capabilitypack's authoritative v2-v4 validation.
func CanonicalCompositePackManifest(raw json.RawMessage) ([]byte, error) {
	var document any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil || ensureEOF(decoder) != nil {
		return nil, errors.New("proposed Pack manifest is malformed")
	}
	object, ok := document.(map[string]any)
	if !ok {
		return nil, errors.New("proposed Pack manifest must be one JSON object")
	}
	schema, ok := object["schema_version"].(json.Number)
	if !ok {
		return nil, errors.New("proposed Pack manifest schema is missing")
	}
	value, err := schema.Int64()
	if err != nil || value < 1 || value > 4 {
		return nil, errors.New("proposed Pack manifest schema is unsupported")
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

type compositeHistoricalResource struct {
	Kind   string         `json:"kind"`
	ID     string         `json:"id"`
	Source string         `json:"source"`
	Files  []FileEvidence `json:"files"`
	SHA256 string         `json:"sha256"`
}

type compositeHistoricalArtifact struct {
	SchemaVersion   int                           `json:"schema_version"`
	PackID          string                        `json:"pack_id"`
	PackVersion     string                        `json:"pack_version"`
	Manifest        FileEvidence                  `json:"manifest"`
	Resources       []compositeHistoricalResource `json:"resources"`
	AggregateSHA256 string                        `json:"aggregate_sha256"`
}

type completeAdmissionSource struct {
	SourceID     string
	Registration SourceConfig
	ProposedLock Lock
}

type completeAdmissionGeneration struct {
	PackID                string
	ProposedVersion       string
	ProposedManifest      json.RawMessage
	ResultingConfigSHA256 string
	LockSetSHA256         string
	Sources               []completeAdmissionSource
}

func materializeCompositeResult(staged string, plan CompositePlan, roots []string, base Config, manifest packManifest) error {
	return materializeCompleteAdmissionResult(staged, completeAdmissionGenerationFromComposite(plan), roots, base, manifest)
}

func completeAdmissionGenerationFromComposite(plan CompositePlan) completeAdmissionGeneration {
	sources := make([]completeAdmissionSource, len(plan.Members))
	for i, member := range plan.Members {
		sources[i] = completeAdmissionSource{SourceID: member.SourceID, Registration: member.Registration, ProposedLock: member.ProposedLock}
	}
	return completeAdmissionGeneration{
		PackID: plan.PackID, ProposedVersion: plan.ProposedVersion, ProposedManifest: plan.ProposedManifest,
		ResultingConfigSHA256: plan.ResultingConfigSHA256, LockSetSHA256: plan.LockSetSHA256, Sources: sources,
	}
}

func materializeCompleteAdmissionResult(staged string, generation completeAdmissionGeneration, roots []string, base Config, manifest packManifest) error {
	registrations := make([]SourceConfig, len(generation.Sources))
	for i, source := range generation.Sources {
		registrations[i] = source.Registration
	}
	config := Config{SchemaVersion: base.SchemaVersion, Sources: append(append([]SourceConfig(nil), base.Sources...), registrations...)}
	encoded, err := canonicalConfigAfterValidation(config)
	if err != nil || hashBytes(encoded) != generation.ResultingConfigSHA256 {
		return errors.New("complete resulting configuration contradicts sealed plan")
	}
	if err := os.WriteFile(filepath.Join(staged, "sources.json"), encoded, 0o644); err != nil {
		return err
	}
	manifestPath := filepath.Join(staged, "packs", generation.PackID, "pack.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(manifestPath, generation.ProposedManifest, 0o644); err != nil {
		return err
	}
	for i, source := range generation.Sources {
		if err := materializeSelectedResources(staged, roots[i], Lock{}, false, source.ProposedLock); err != nil {
			return err
		}
		if err := writeCanonicalLock(filepath.Join(staged, "sources", source.SourceID+".lock.json"), source.ProposedLock); err != nil {
			return err
		}
	}
	stagedConfig, err := LoadConfig(bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	stagedLocks, err := loadSourceLockSet(staged, stagedConfig)
	if err != nil || stagedLocks.LockSetSHA256 != generation.LockSetSHA256 {
		return errors.New("complete staged lock set contradicts sealed plan")
	}
	return materializePackHistory(staged, generation.PackID, generation.ProposedVersion, generation.ProposedManifest, manifest)
}

type completeAdmissionTransaction struct {
	PlanID             string
	ResultBundleSHA256 string
	Marker             recoveryMarker
}

func (engine Engine) applyCompleteAdmissionTransaction(ctx context.Context, repositoryRoot string, generation completeAdmissionGeneration, roots []string, base Config, manifest packManifest, transaction completeAdmissionTransaction) (ApplyResult, error) {
	bundle := filepath.Join(repositoryRoot, "bundle")
	staged, backup := transactionPaths(repositoryRoot, transaction.PlanID)
	markerPath := recoveryMarkerPath(repositoryRoot)
	for _, path := range []string{staged, backup, markerPath} {
		if _, err := os.Lstat(path); err == nil || !errors.Is(err, fs.ErrNotExist) {
			return ApplyResult{}, fmt.Errorf("%w: unexpected transaction path %s", ErrRecoveryEvidence, path)
		}
	}
	if err := copyTreeExact(bundle, staged); err != nil {
		return ApplyResult{}, fmt.Errorf("stage complete bundle: %w", err)
	}
	cleanupStaged := true
	defer func() {
		if cleanupStaged {
			_ = os.RemoveAll(staged)
		}
	}()
	if err := materializeCompleteAdmissionResult(staged, generation, roots, base, manifest); err != nil {
		return ApplyResult{}, err
	}
	if err := engine.Validate.ValidateBundle(ctx, repositoryRoot, staged); err != nil {
		return ApplyResult{}, fmt.Errorf("validate complete staged admission bundle: %w", err)
	}
	oldHash, err := treeHash(bundle)
	if err != nil {
		return ApplyResult{}, err
	}
	newHash, err := treeHash(staged)
	if err != nil {
		return ApplyResult{}, err
	}
	if newHash != transaction.ResultBundleSHA256 {
		return ApplyResult{}, errors.New("complete staged result tree contradicts sealed Check result")
	}
	if err := engine.inject(FaultBeforeSwap); err != nil {
		return ApplyResult{}, err
	}
	marker := transaction.Marker
	marker.SchemaVersion, marker.PlanID, marker.Phase = recoveryMarkerSchema, transaction.PlanID, "prepared"
	marker.Bundle, marker.Backup, marker.Staged = bundle, backup, staged
	marker.OldSHA256, marker.NewSHA256 = oldHash, newHash
	if err := writeRecoveryMarker(markerPath, &marker); err != nil {
		return ApplyResult{}, err
	}
	if err := os.Rename(bundle, backup); err != nil {
		_ = os.Remove(markerPath)
		return ApplyResult{}, fmt.Errorf("first bundle rename: %w", err)
	}
	if err := syncDirectory(repositoryRoot); err != nil {
		return ApplyResult{}, err
	}
	cleanupStaged = false
	marker.Phase = "old-renamed"
	if err := writeRecoveryMarker(markerPath, &marker); err != nil {
		return ApplyResult{}, err
	}
	if err := engine.inject(FaultAfterFirstRename); err != nil {
		return ApplyResult{}, err
	}
	if err := os.Rename(staged, bundle); err != nil {
		return ApplyResult{}, fmt.Errorf("second bundle rename: %w", err)
	}
	if err := syncDirectory(repositoryRoot); err != nil {
		return ApplyResult{}, err
	}
	marker.Phase = "new-installed"
	if err := writeRecoveryMarker(markerPath, &marker); err != nil {
		return ApplyResult{}, err
	}
	if err := engine.inject(FaultAfterSecondRename); err != nil {
		return ApplyResult{}, err
	}
	if err := verifyTreeHash(bundle, newHash); err != nil {
		return ApplyResult{}, err
	}
	marker.Phase = "cleanup"
	if err := writeRecoveryMarker(markerPath, &marker); err != nil {
		return ApplyResult{}, err
	}
	if err := engine.inject(FaultDuringCleanup); err != nil {
		return ApplyResult{}, err
	}
	if err := cleanupCommitted(marker); err != nil {
		return ApplyResult{}, err
	}
	if err := os.Remove(markerPath); err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{Status: "applied", PlanID: transaction.PlanID, Changed: true}, nil
}

func materializePackHistory(staged, packID, version string, manifestBytes []byte, manifest packManifest) error {
	history := filepath.Join(staged, "history", packID, version)
	if _, err := os.Lstat(history); err == nil || !errors.Is(err, fs.ErrNotExist) {
		return errors.New("initial Pack history generation already exists or is unsafe")
	}
	if err := os.MkdirAll(history, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(history, "pack.json"), manifestBytes, 0o644); err != nil {
		return err
	}
	artifact := compositeHistoricalArtifact{SchemaVersion: 1, PackID: packID, PackVersion: version}
	manifestFiles, err := inventory(filepath.Join(history, "pack.json"))
	if err != nil || len(manifestFiles) != 1 {
		return errors.New("inspect proposed historical manifest")
	}
	artifact.Manifest = manifestFiles[0]
	artifact.Manifest.Path = "pack.json"
	for _, resource := range manifest.Resources {
		if resource.Source == "" {
			files := []FileEvidence{}
			artifact.Resources = append(artifact.Resources, compositeHistoricalResource{Kind: resource.Kind, ID: resource.ID, Source: "", Files: files, SHA256: resourceHash(files)})
			continue
		}
		source := filepath.Join(staged, filepath.FromSlash(resource.Source))
		target := filepath.Join(history, filepath.FromSlash(resource.Source))
		if err := copyTreeExact(source, target); err != nil {
			return fmt.Errorf("retain initial historical resource %s/%s: %w", resource.Kind, resource.ID, err)
		}
		files, err := inventory(target)
		if err != nil {
			return err
		}
		for i := range files {
			relative, err := filepath.Rel(history, filepath.Join(target, filepath.FromSlash(files[i].Path)))
			if err != nil {
				return err
			}
			files[i].Path = filepath.ToSlash(relative)
		}
		artifact.Resources = append(artifact.Resources, compositeHistoricalResource{Kind: resource.Kind, ID: resource.ID, Source: resource.Source, Files: files, SHA256: resourceHash(files)})
	}
	artifact.AggregateSHA256 = compositeHistoricalAggregate(artifact)
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(history, "artifact.json"), append(data, '\n'), 0o644)
}

func compositeHistoricalAggregate(artifact compositeHistoricalArtifact) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "%d\x00%s\x00%s\n", artifact.SchemaVersion, artifact.PackID, artifact.PackVersion)
	fmt.Fprintf(hash, "manifest\x00%s\x00%d\x00%04o\x00%s\n", artifact.Manifest.Path, artifact.Manifest.Size, artifact.Manifest.Mode, artifact.Manifest.SHA256)
	for _, resource := range artifact.Resources {
		fmt.Fprintf(hash, "%s\x00%s\x00%s\x00%s\n", resource.Kind, resource.ID, resource.Source, resource.SHA256)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func sealCompositePlan(plan CompositePlan) (string, error) {
	plan.PlanID = ""
	data, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	return "pack-sync-" + hashBytes(data), nil
}

func (engine Engine) withCompositeSnapshots(ctx context.Context, members []CompositeRegistrationMember, candidates []Candidate, base string, roots []string, index int, visit func() error) error {
	if index == len(members) {
		return visit()
	}
	dir := filepath.Join(base, fmt.Sprintf("%03d-%s", index, members[index].Registration.ID))
	if err := os.Mkdir(dir, 0o700); err != nil {
		return err
	}
	defer os.Remove(dir)
	return engine.Source.WithSnapshot(ctx, candidates[index], dir, func(root string) error {
		roots[index] = root
		return engine.withCompositeSnapshots(ctx, members, candidates, base, roots, index+1, visit)
	})
}

func (engine Engine) ApplyComposite(ctx context.Context, request CompositeApplyRequest) (ApplyResult, error) {
	if engine.Source == nil || engine.Validate == nil || !request.Plan.VerifySeal() || request.Plan.Status != "review-required" {
		return ApplyResult{}, errors.New("composite Apply requires acquisition, validation, and an applicable exact sealed plan")
	}
	members, digest, err := CanonicalRegistrationBundle(request.Members)
	if err != nil || digest != request.Plan.RegistrationBundleSHA256 || request.PackID != request.Plan.PackID || request.ProposedVersion != request.Plan.ProposedVersion {
		return ApplyResult{}, errors.New("composite registration changed after Check")
	}
	_, manifestBytes, err := validateCompositeManifest(request.PackID, request.ProposedVersion, request.ProposedManifest)
	if err != nil || !bytes.Equal(manifestBytes, request.Plan.ProposedManifest) || hashBytes(manifestBytes) != request.Plan.ProposedManifestSHA256 {
		return ApplyResult{}, errors.New("proposed Pack generation changed after Check")
	}
	if !reflect.DeepEqual(members, compositePlanMembers(request.Plan)) {
		return ApplyResult{}, errors.New("composite member facts changed after Check")
	}
	if err := ValidateCompositeClassificationEvidence(request.Plan, request.ClassificationEvidence); err != nil {
		return ApplyResult{}, err
	}
	if err := requireEmptyDirectory(request.AcquisitionDir); err != nil {
		return ApplyResult{}, err
	}
	candidates := make([]Candidate, len(members))
	for i, member := range members {
		candidates[i], err = engine.Source.ResolveCommit(ctx, member.Registration, member.Registration.Selector.Ref)
		if err != nil {
			return ApplyResult{}, fmt.Errorf("re-resolve member %s: %w", member.Registration.ID, err)
		}
		if !reflect.DeepEqual(candidates[i], request.Plan.Members[i].Candidate) {
			return ApplyResult{}, fmt.Errorf("member %s candidate changed after Check", member.Registration.ID)
		}
	}
	roots := make([]string, len(members))
	var result ApplyResult
	err = engine.withCompositeSnapshots(ctx, members, candidates, request.AcquisitionDir, roots, 0, func() error {
		for i := range roots {
			if err := verifySnapshot(roots[i], request.Plan.Members[i].ProposedLock); err != nil {
				return fmt.Errorf("reacquired member %s changed: %w", members[i].Registration.ID, err)
			}
		}
		guard, err := bundletransaction.Acquire(ctx, request.RepositoryRoot)
		if err != nil {
			return err
		}
		defer guard.Release()
		for i, member := range members {
			if err := validateCompositeLegalAdmission(request.RepositoryRoot, member, candidates[i]); err != nil {
				return fmt.Errorf("source %s legal admission changed after Check: %w", member.Registration.ID, err)
			}
		}
		result, err = engine.applyCompositeLocked(ctx, request, members, roots)
		return err
	})
	if err != nil {
		return ApplyResult{}, err
	}
	if err := requireEmptyDirectory(request.AcquisitionDir); err != nil {
		return ApplyResult{}, err
	}
	return result, nil
}

func compositePlanMembers(plan CompositePlan) []CompositeRegistrationMember {
	result := make([]CompositeRegistrationMember, len(plan.Members))
	for i, member := range plan.Members {
		result[i] = CompositeRegistrationMember{Registration: member.Registration, LegalAdmission: member.LegalAdmission}
	}
	return result
}

// RevalidateCompositeCandidates freshly resolves every exact member candidate
// without acquiring snapshots or writing. Publication calls it around its
// first write boundary; Apply remains the owner of complete reacquisition.
func (engine Engine) RevalidateCompositeCandidates(ctx context.Context, plan CompositePlan) error {
	if engine.Source == nil || !plan.VerifySeal() || len(plan.Members) < 2 || len(plan.SourceIDs) != len(plan.Members) {
		return errors.New("fresh composite provenance revalidation requires a source and exact sealed complete plan")
	}
	for i, member := range plan.Members {
		if plan.SourceIDs[i] != member.SourceID || member.SourceID != member.Registration.ID ||
			member.Registration.Selector.Mode != SelectorCommit {
			return errors.New("sealed composite member order or exact selector is invalid")
		}
		candidate, err := engine.Source.ResolveCommit(ctx, member.Registration, member.Registration.Selector.Ref)
		if err != nil {
			return fmt.Errorf("re-resolve composite member %s: %w", member.SourceID, err)
		}
		if !reflect.DeepEqual(candidate, member.Candidate) {
			return fmt.Errorf("composite member %s candidate provenance changed after validation", member.SourceID)
		}
	}
	return nil
}

func (engine Engine) applyCompositeLocked(ctx context.Context, request CompositeApplyRequest, members []CompositeRegistrationMember, roots []string) (ApplyResult, error) {
	plan := request.Plan
	if !plan.VerifySeal() {
		return ApplyResult{}, errors.New("composite plan changed while acquiring transaction lock")
	}
	if err := ValidateCompositeClassificationEvidence(plan, request.ClassificationEvidence); err != nil {
		return ApplyResult{}, err
	}
	current, err := readCompositeLocalUnlocked(request.RepositoryRoot, members, plan.PackID, false)
	if err != nil {
		return ApplyResult{}, err
	}
	base, err := repositoryBase(request.RepositoryRoot)
	if err != nil || base != plan.Preconditions.BaseCommit || hashBytes(current.configBytes) != plan.Preconditions.ConfigSHA256 || current.lockSet.LockSetSHA256 != plan.Preconditions.LockSetSHA256 {
		return ApplyResult{}, errors.New("stale composite plan: local authority changed after Check")
	}
	_, manifestsHash, err := loadManifests(request.RepositoryRoot)
	if err != nil || manifestsHash != plan.Preconditions.ManifestsSHA256 {
		return ApplyResult{}, errors.New("stale composite plan: manifests changed after Check")
	}
	bundle := filepath.Join(request.RepositoryRoot, "bundle")
	if hash, err := treeHash(bundle); err != nil || hash != plan.Preconditions.BundleSHA256 {
		return ApplyResult{}, errors.New("stale composite plan: complete bundle changed after Check")
	}
	manifest, _, err := validateCompositeManifest(plan.PackID, plan.ProposedVersion, plan.ProposedManifest)
	if err != nil {
		return ApplyResult{}, err
	}
	return engine.applyCompleteAdmissionTransaction(ctx, request.RepositoryRoot, completeAdmissionGenerationFromComposite(plan), roots, current.config, manifest, completeAdmissionTransaction{
		PlanID: plan.PlanID, ResultBundleSHA256: plan.ResultBundleSHA256,
		Marker: recoveryMarker{
			SourceID: plan.SourceIDs[0], SourceLockSHA256: plan.Members[0].SourceLockSHA256,
			LockSetSHA256: plan.LockSetSHA256, Operation: "register_bundle",
			SourceIDs: append([]string(nil), plan.SourceIDs...), RegistrationBundleSHA256: plan.RegistrationBundleSHA256,
		},
	})
}
