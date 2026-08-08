package packsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/yersonargotev/packy/internal/bundletransaction"
	"github.com/yersonargotev/packy/internal/capabilitypack"
)

// SingleSourceAdmissionCheckRequest is the complete v2.3 registration intent
// for one absent Pack and one absent Pack Source.
type SingleSourceAdmissionCheckRequest struct {
	RepositoryRoot         string
	AcquisitionDir         string
	Registration           SourceConfig
	RegistrationSHA256     string
	ProposedVersion        string
	ProposedManifest       json.RawMessage
	ProposedManifestSHA256 string
	LegalAdmission         CompositeLegalAdmission
}

// SingleSourceAdmissionPlan seals the complete observable initial generation.
// It is proposal evidence only and grants no publication authority.
type SingleSourceAdmissionPlan struct {
	SchemaVersion          int                     `json:"schema_version"`
	PlanID                 string                  `json:"plan_id"`
	Status                 string                  `json:"status"`
	PackID                 string                  `json:"pack_id"`
	ProposedVersion        string                  `json:"proposed_version"`
	ProposedManifest       json.RawMessage         `json:"proposed_manifest"`
	ProposedManifestSHA256 string                  `json:"proposed_manifest_sha256"`
	Registration           SourceConfig            `json:"registration"`
	RegistrationSHA256     string                  `json:"registration_sha256"`
	Candidate              Candidate               `json:"candidate"`
	LegalAdmission         CompositeLegalAdmission `json:"legal_admission"`
	Classification         PackImpact              `json:"classification"`
	Preconditions          Preconditions           `json:"preconditions"`
	ProposedLock           Lock                    `json:"proposed_lock"`
	SourceLockSHA256       string                  `json:"source_lock_sha256"`
	LockSetSHA256          string                  `json:"lock_set_sha256"`
	ResultingConfigSHA256  string                  `json:"resulting_config_sha256"`
	ResultBundleSHA256     string                  `json:"result_bundle_sha256"`
}

func (plan SingleSourceAdmissionPlan) VerifySeal() bool {
	want, err := sealSingleSourceAdmissionPlan(plan)
	return err == nil && want == plan.PlanID
}

func (plan SingleSourceAdmissionPlan) CanonicalJSON() ([]byte, error) {
	if !plan.VerifySeal() {
		return nil, errors.New("single-source admission plan is not sealed")
	}
	encoded, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

// CheckSingleSourceAdmission inspects and validates a complete initial Pack
// generation in disposable state. It never writes repository bundle state.
func (engine Engine) CheckSingleSourceAdmission(ctx context.Context, request SingleSourceAdmissionCheckRequest) (SingleSourceAdmissionPlan, error) {
	if engine.Source == nil || engine.Validate == nil || request.RepositoryRoot == "" || request.AcquisitionDir == "" || request.ProposedVersion == "" {
		return SingleSourceAdmissionPlan{}, errors.New("single-source admission Check requires acquisition, validation, repository root, acquisition directory, and proposed version")
	}
	if err := requireEmptyDirectory(request.AcquisitionDir); err != nil {
		return SingleSourceAdmissionPlan{}, fmt.Errorf("acquisition directory: %w", err)
	}
	registration, registrationDigest, err := canonicalRegistration(request.Registration)
	if err != nil || registrationDigest != request.RegistrationSHA256 {
		return SingleSourceAdmissionPlan{}, errors.New("registration is malformed or its canonical digest does not match")
	}
	if registration.Selector.Mode != SelectorStableRelease {
		return SingleSourceAdmissionPlan{}, errors.New("initial single-source admission requires the latest stable release selector")
	}
	packIDs := bindingPackIDs(registration.Resources)
	if len(packIDs) != 1 || len(registration.Resources) == 0 {
		return SingleSourceAdmissionPlan{}, errors.New("initial single-source admission must bind exactly one Pack")
	}
	packID := packIDs[0]
	manifest, manifestBytes, err := validateSingleSourceAdmissionManifest(request.RepositoryRoot, packID, request.ProposedVersion, request.ProposedManifest)
	if err != nil {
		return SingleSourceAdmissionPlan{}, fmt.Errorf("proposed manifest: %w", err)
	}
	if hashBytes(manifestBytes) != request.ProposedManifestSHA256 {
		return SingleSourceAdmissionPlan{}, errors.New("proposed manifest canonical digest does not match")
	}
	if !singleSourceBindingsMatchManifest(registration.Resources, manifest.Resources) {
		return SingleSourceAdmissionPlan{}, errors.New("source bindings and proposed manifest resources must match bidirectionally")
	}
	member := CompositeRegistrationMember{Registration: registration, LegalAdmission: request.LegalAdmission}
	if !fullDigest(member.LegalAdmission.EvidenceSHA256) || member.LegalAdmission.EvidenceReference == "" || member.LegalAdmission.Disposition != RedistributableDisposition {
		return SingleSourceAdmissionPlan{}, errors.New("initial single-source admission requires complete redistributable legal evidence")
	}
	initial, err := readCompositeLocal(request.RepositoryRoot, []CompositeRegistrationMember{member}, packID)
	if err != nil {
		return SingleSourceAdmissionPlan{}, err
	}
	_, initialManifestsHash, err := loadManifests(request.RepositoryRoot)
	if err != nil {
		return SingleSourceAdmissionPlan{}, err
	}
	initialBundleHash, err := treeHash(filepath.Join(request.RepositoryRoot, "bundle"))
	if err != nil {
		return SingleSourceAdmissionPlan{}, err
	}
	initialBaseCommit, err := repositoryBase(request.RepositoryRoot)
	if err != nil {
		return SingleSourceAdmissionPlan{}, err
	}
	releases, err := engine.Source.Releases(ctx, registration)
	if err != nil {
		return SingleSourceAdmissionPlan{}, fmt.Errorf("list published releases: %w", err)
	}
	candidate, err := engine.resolveFromReleases(ctx, registration, registration.Selector, releases)
	if err != nil {
		return SingleSourceAdmissionPlan{}, err
	}
	if blockers := validateCandidate(registration, candidate, registration.Selector); len(blockers) != 0 {
		return SingleSourceAdmissionPlan{}, fmt.Errorf("candidate is invalid: %v", blockers)
	}
	if err := validateCompositeLegalAdmission(request.RepositoryRoot, member, candidate); err != nil {
		return SingleSourceAdmissionPlan{}, fmt.Errorf("legal admission: %w", err)
	}

	var result SingleSourceAdmissionPlan
	err = engine.Source.WithSnapshot(ctx, candidate, request.AcquisitionDir, func(snapshotRoot string) error {
		guard, err := bundletransaction.Acquire(ctx, request.RepositoryRoot)
		if err != nil {
			return err
		}
		defer guard.Release()
		fresh, err := readCompositeLocalUnlocked(request.RepositoryRoot, []CompositeRegistrationMember{member}, packID)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(initial, fresh) {
			return errors.New("local bundle facts changed during single-source admission Check; retry")
		}
		manifests, manifestsHash, err := loadManifests(request.RepositoryRoot)
		if err != nil {
			return err
		}
		bundleDigest, err := treeHash(filepath.Join(request.RepositoryRoot, "bundle"))
		if err != nil {
			return err
		}
		baseCommit, err := repositoryBase(request.RepositoryRoot)
		if err != nil {
			return err
		}
		if manifestsHash != initialManifestsHash || bundleDigest != initialBundleHash || baseCommit != initialBaseCommit {
			return errors.New("local bundle base facts changed during single-source admission Check; retry")
		}
		manifests[packID] = manifest
		bindings, blockers := deriveDestinations(registration.Resources, manifests)
		if len(blockers) != 0 {
			return fmt.Errorf("source bindings are invalid: %v", blockers)
		}
		basePlan := Plan{Registration: &registration, Candidate: candidate, Selector: registration.Selector}
		if err := buildPlan(snapshotRoot, request.RepositoryRoot, registration, bindings, manifests, Lock{}, false, initial.existingPacks, buildPlanCheck, &basePlan); err != nil {
			return err
		}
		if len(basePlan.Blockers) != 0 || len(basePlan.AffectedPacks) != 1 {
			return fmt.Errorf("initial single-source admission is blocked: %v", basePlan.Blockers)
		}
		lockBytes, lockDigest, err := CanonicalSourceLock(basePlan.ProposedLock)
		if err != nil || len(lockBytes) == 0 {
			return err
		}
		lockDigests := make([]SourceLockDigest, 0, len(initial.lockSet.Digests)+1)
		for id, digest := range initial.lockSet.Digests {
			lockDigests = append(lockDigests, SourceLockDigest{SourceID: id, SHA256: digest})
		}
		lockDigests = append(lockDigests, SourceLockDigest{SourceID: registration.ID, SHA256: lockDigest})
		lockSetDigest, err := LockSetSHA256(lockDigests)
		if err != nil {
			return err
		}
		resultConfig := Config{SchemaVersion: initial.config.SchemaVersion, Sources: append(append([]SourceConfig(nil), initial.config.Sources...), registration)}
		configBytes, err := canonicalConfigAfterValidation(resultConfig)
		if err != nil {
			return err
		}
		result = SingleSourceAdmissionPlan{
			SchemaVersion: 1, Status: "review-required", PackID: packID,
			ProposedVersion: request.ProposedVersion, ProposedManifest: manifestBytes,
			ProposedManifestSHA256: hashBytes(manifestBytes), Registration: registration,
			RegistrationSHA256: registrationDigest, Candidate: candidate,
			LegalAdmission: member.LegalAdmission, Classification: basePlan.AffectedPacks[0],
			Preconditions: Preconditions{BaseCommit: baseCommit, ConfigSHA256: hashBytes(initial.configBytes), ManifestsSHA256: manifestsHash, BundleSHA256: bundleDigest, LockSetSHA256: initial.lockSet.LockSetSHA256},
			ProposedLock:  basePlan.ProposedLock, SourceLockSHA256: lockDigest,
			LockSetSHA256: lockSetDigest, ResultingConfigSHA256: hashBytes(configBytes),
		}
		disposable := filepath.Join(request.AcquisitionDir, "complete-result")
		if err := copyTreeExact(filepath.Join(request.RepositoryRoot, "bundle"), disposable); err != nil {
			return err
		}
		defer os.RemoveAll(disposable)
		composite := result.asCompositePlan()
		if err := materializeCompositeResult(disposable, composite, []string{snapshotRoot}, initial.config, manifest); err != nil {
			return err
		}
		if err := engine.Validate.ValidateBundle(ctx, request.RepositoryRoot, disposable); err != nil {
			return fmt.Errorf("validate disposable complete single-source result: %w", err)
		}
		result.ResultBundleSHA256, err = treeHash(disposable)
		if err != nil {
			return err
		}
		result.PlanID, err = sealSingleSourceAdmissionPlan(result)
		return err
	})
	if err != nil {
		return SingleSourceAdmissionPlan{}, fmt.Errorf("inspect single-source admission snapshot: %w", err)
	}
	if err := requireEmptyDirectory(request.AcquisitionDir); err != nil {
		return SingleSourceAdmissionPlan{}, fmt.Errorf("acquisition did not clean caller-supplied directory: %w", err)
	}
	return result, nil
}

func singleSourceBindingsMatchManifest(bindings []Binding, resources []manifestResource) bool {
	want := make(map[string]bool, len(bindings))
	for _, binding := range bindings {
		want[binding.Kind+"\x00"+binding.ResourceID] = true
	}
	got := make(map[string]bool, len(resources))
	for _, resource := range resources {
		got[resource.Kind+"\x00"+resource.ID] = true
	}
	return reflect.DeepEqual(want, got)
}

func validateSingleSourceAdmissionManifest(repositoryRoot, packID, version string, raw json.RawMessage) (packManifest, []byte, error) {
	canonical, err := canonicalJSON(raw)
	if err != nil {
		return packManifest{}, nil, err
	}
	var marker struct {
		Selectable *bool `json:"selectable"`
	}
	var manifest packManifest
	if json.Unmarshal(canonical, &marker) != nil || json.Unmarshal(canonical, &manifest) != nil || marker.Selectable == nil {
		return packManifest{}, nil, errors.New("initial admission requires one complete current Pack manifest")
	}
	manifest.SchemaVersion = 4
	if manifest.ID != packID || manifest.Version != version || len(manifest.Resources) == 0 {
		return packManifest{}, nil, errors.New("proposed manifest identity, version, or resources contradict registration")
	}
	temporary, err := os.MkdirTemp("", "packy-single-source-manifest-")
	if err != nil {
		return packManifest{}, nil, err
	}
	defer os.RemoveAll(temporary)
	name := filepath.Join(temporary, "pack.json")
	if err := os.WriteFile(name, canonical, 0o600); err != nil {
		return packManifest{}, nil, err
	}
	pack, err := capabilitypack.LoadCurrentManifest(name, filepath.Join(repositoryRoot, "bundle"), false)
	if err != nil || pack.ID != packID || pack.Version != version {
		return packManifest{}, nil, fmt.Errorf("proposed manifest disagrees with the current Pack contract: %w", err)
	}
	manifest.canonicalV4 = append([]byte(nil), canonical...)
	return manifest, canonical, nil
}

func canonicalJSON(raw []byte) ([]byte, error) {
	var document any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil || ensureEOF(decoder) != nil {
		return nil, errors.New("JSON document is malformed")
	}
	if _, ok := document.(map[string]any); !ok {
		return nil, errors.New("JSON document must be one object")
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func sealSingleSourceAdmissionPlan(plan SingleSourceAdmissionPlan) (string, error) {
	plan.PlanID = ""
	encoded, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	return "pack-sync-" + hashBytes(encoded), nil
}

func (plan SingleSourceAdmissionPlan) asCompositePlan() CompositePlan {
	return CompositePlan{
		SchemaVersion: 1, Status: plan.Status, PackID: plan.PackID,
		ProposedVersion: plan.ProposedVersion, ProposedManifest: plan.ProposedManifest,
		ProposedManifestSHA256: plan.ProposedManifestSHA256,
		SourceIDs:              []string{plan.Registration.ID},
		Members:                []CompositeMemberPlan{{SourceID: plan.Registration.ID, Registration: plan.Registration, Candidate: plan.Candidate, LegalAdmission: plan.LegalAdmission, SourceLockSHA256: plan.SourceLockSHA256, ProposedLock: plan.ProposedLock}},
		Preconditions:          plan.Preconditions, ResultingConfigSHA256: plan.ResultingConfigSHA256,
		LockSetSHA256: plan.LockSetSHA256, ResultBundleSHA256: plan.ResultBundleSHA256,
	}
}
