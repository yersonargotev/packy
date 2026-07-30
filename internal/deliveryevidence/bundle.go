// Package deliveryevidence owns the durable, repository-scoped authority used
// while delivering one qualified issue. It deliberately contains no GitHub or
// Git process integration.
package deliveryevidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	SchemaV1 = "packy.issue-delivery/v1"
	SchemaV2 = "packy.issue-delivery/v2"
)

type RepositoryIdentity struct {
	Owner  string `json:"owner"`
	Name   string `json:"name"`
	NodeID string `json:"node_id"`
}
type IssueIdentity struct {
	Number int    `json:"number"`
	NodeID string `json:"node_id"`
}
type SpecIdentity struct {
	Number int    `json:"number"`
	NodeID string `json:"node_id"`
}
type Authority struct {
	Kind                  DeliveryAuthorityKind   `json:"kind,omitempty"`
	IssueSHA256           string                  `json:"issue_sha256"`
	SpecSHA256            string                  `json:"spec_sha256,omitempty"`
	Labels                []string                `json:"labels"`
	DependencyDisposition []DependencyDisposition `json:"dependency_disposition"`
	AcceptanceCriteria    []string                `json:"acceptance_criteria"`
}
type DeliveryAuthorityKind string

const (
	AuthoritySelfContainedIssue     DeliveryAuthorityKind = "self-contained-issue"
	AuthorityIssueWithSpecification DeliveryAuthorityKind = "issue-with-specification"
)

type DeliveryRiskProfile string

const (
	RiskLow      DeliveryRiskProfile = "low-risk"
	RiskStandard DeliveryRiskProfile = "standard"
	RiskHigh     DeliveryRiskProfile = "high-risk"
)

type QualificationInput struct {
	Schema        string
	IssueNumber   int
	SpecNumber    int
	AuthorityKind DeliveryAuthorityKind
	RiskProfile   DeliveryRiskProfile
}

type QualificationPlan struct {
	AuthorityKind    DeliveryAuthorityKind
	RiskProfile      DeliveryRiskProfile
	HasSpecification bool
}

func CompileQualification(input QualificationInput) (QualificationPlan, error) {
	switch input.Schema {
	case SchemaV1:
		if input.IssueNumber <= 0 || input.SpecNumber <= 0 || input.IssueNumber == input.SpecNumber ||
			input.AuthorityKind != "" || input.RiskProfile != "" {
			return QualificationPlan{}, errors.New("v1 qualification requires distinct positive issue/spec numbers and forbids v2 fields")
		}
		return QualificationPlan{HasSpecification: true}, nil
	case SchemaV2:
		if input.IssueNumber <= 0 {
			return QualificationPlan{}, errors.New("v2 qualification requires a positive issue number")
		}
		profile := input.RiskProfile
		if profile == "" {
			profile = RiskStandard
		}
		switch profile {
		case RiskLow, RiskStandard, RiskHigh:
		default:
			return QualificationPlan{}, errors.New("v2 qualification risk profile must be low-risk, standard, or high-risk")
		}
		switch input.AuthorityKind {
		case AuthoritySelfContainedIssue:
			if input.SpecNumber != 0 {
				return QualificationPlan{}, errors.New("self-contained issue qualification forbids a specification number")
			}
			return QualificationPlan{AuthorityKind: input.AuthorityKind, RiskProfile: profile}, nil
		case AuthorityIssueWithSpecification:
			if input.SpecNumber <= 0 || input.SpecNumber == input.IssueNumber {
				return QualificationPlan{}, errors.New("issue-with-specification qualification requires a distinct positive specification number")
			}
			return QualificationPlan{AuthorityKind: input.AuthorityKind, RiskProfile: profile, HasSpecification: true}, nil
		default:
			return QualificationPlan{}, errors.New("v2 qualification authority kind is required")
		}
	default:
		return QualificationPlan{}, fmt.Errorf("unsupported qualification schema %q", input.Schema)
	}
}

func ValidateLegacyWorkflowBundle(bundle Bundle) error {
	if bundle.Schema != SchemaV1 {
		return errors.New("schema v2 requires Advance; legacy delivery commands accept only schema v1")
	}
	return nil
}

type DependencyDisposition struct {
	Identity    string                     `json:"identity"`
	Disposition DependencyDispositionState `json:"disposition"`
}
type DependencyDispositionState string

const (
	DependencyBlocking  DependencyDispositionState = "blocking"
	DependencySatisfied DependencyDispositionState = "satisfied"
)

type LedgerEntry struct {
	Identity     string `json:"identity"`
	Requirement  string `json:"requirement"`
	EvidenceLink string `json:"evidence_link"`
}
type DeferredEntry struct {
	Identity     string `json:"identity"`
	Requirement  string `json:"requirement"`
	EvidenceLink string `json:"evidence_link"`
	Owner        string `json:"owner"`
}
type PrerequisiteEntry struct {
	Identity          string `json:"identity"`
	Requirement       string `json:"requirement"`
	EvidenceLink      string `json:"evidence_link"`
	Disposition       string `json:"disposition"`
	ExceptionBoundary string `json:"exception_boundary"`
}
type ScopeLedger struct {
	OwnedNow      []LedgerEntry       `json:"owned_now"`
	Deferred      []DeferredEntry     `json:"deferred"`
	Forbidden     []LedgerEntry       `json:"forbidden"`
	Prerequisites []PrerequisiteEntry `json:"prerequisites"`
}
type AcceptanceRow struct {
	Identity              string          `json:"identity"`
	Criterion             string          `json:"criterion"`
	OwningSeam            string          `json:"owning_seam"`
	PositiveEvidence      string          `json:"positive_evidence"`
	NegativeEvidence      string          `json:"negative_evidence"`
	FailureEvidence       string          `json:"failure_evidence"`
	MutationEvidence      string          `json:"mutation_evidence"`
	CompatibilityEvidence string          `json:"compatibility_evidence"`
	PreservationEvidence  string          `json:"preservation_evidence"`
	MigrationEvidence     string          `json:"migration_evidence"`
	State                 AcceptanceState `json:"state"`
}
type AcceptanceState string

const (
	AcceptancePlanned     AcceptanceState = "planned"
	AcceptanceImplemented AcceptanceState = "implemented"
	AcceptanceProved      AcceptanceState = "proved"
)

type Iteration struct {
	Sequence       int    `json:"sequence"`
	Identity       string `json:"identity"`
	BaseSHA        string `json:"base_sha"`
	HeadSHA        string `json:"head_sha"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}
type Bundle struct {
	Schema             string                      `json:"schema"`
	Repository         RepositoryIdentity          `json:"repository"`
	Issue              IssueIdentity               `json:"issue"`
	Spec               SpecIdentity                `json:"spec"`
	Authority          Authority                   `json:"authority"`
	RiskProfile        DeliveryRiskProfile         `json:"risk_profile,omitempty"`
	Scope              ScopeLedger                 `json:"scope"`
	AcceptanceMatrix   []AcceptanceRow             `json:"acceptance_matrix"`
	StartingBaseSHA    string                      `json:"starting_base_sha"`
	Iterations         []Iteration                 `json:"iterations"`
	ReviewReceipts     []ReviewReceipt             `json:"review_receipts"`
	Adjudications      []Adjudication              `json:"adjudications"`
	ValidationReceipts []ValidationReceipt         `json:"validation_receipts,omitempty"`
	FocusedValidation  []FocusedValidationEvidence `json:"focused_validation,omitempty"`
}

// TypedObservationHash binds a kind and identity to canonical transient facts.
// Authority body bytes may be hashed but are never retained by Bundle. Callers
// must exclude credentials and environment material.
func TypedObservationHash(kind, identity string, facts any) (string, error) {
	if strings.TrimSpace(kind) == "" || strings.TrimSpace(identity) == "" {
		return "", errors.New("observation kind and identity are required")
	}
	v := struct {
		Kind     string `json:"kind"`
		Identity string `json:"identity"`
		Facts    any    `json:"facts"`
	}{kind, identity, facts}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

func CanonicalJSON(bundle Bundle) ([]byte, error) {
	bundle.Authority.Labels = clone(bundle.Authority.Labels)
	bundle.Authority.DependencyDisposition = clone(bundle.Authority.DependencyDisposition)
	bundle.Authority.AcceptanceCriteria = clone(bundle.Authority.AcceptanceCriteria)
	bundle.Scope.OwnedNow = clone(bundle.Scope.OwnedNow)
	bundle.Scope.Deferred = clone(bundle.Scope.Deferred)
	bundle.Scope.Forbidden = clone(bundle.Scope.Forbidden)
	bundle.Scope.Prerequisites = clone(bundle.Scope.Prerequisites)
	bundle.AcceptanceMatrix = clone(bundle.AcceptanceMatrix)
	bundle.Iterations = clone(bundle.Iterations)
	bundle.ReviewReceipts = clone(bundle.ReviewReceipts)
	for i := range bundle.ReviewReceipts {
		bundle.ReviewReceipts[i].Findings = clone(bundle.ReviewReceipts[i].Findings)
	}
	bundle.Adjudications = clone(bundle.Adjudications)
	bundle.ValidationReceipts = clone(bundle.ValidationReceipts)
	bundle.FocusedValidation = clone(bundle.FocusedValidation)
	canonicalize(&bundle)
	if err := Validate(bundle); err != nil {
		return nil, err
	}
	b, err := marshalBundle(bundle)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func Digest(bundle Bundle) (string, error) {
	b, err := CanonicalJSON(bundle)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

// Decode accepts exactly one canonical JSON value and fails closed on schema
// additions. Non-canonical encodings are rejected so bytes and meaning agree.
func Decode(data []byte) (Bundle, error) {
	var b Bundle
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&b); err != nil {
		return b, fmt.Errorf("decode delivery evidence: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		if err == nil {
			return b, errors.New("decode delivery evidence: multiple JSON values")
		}
		return b, fmt.Errorf("decode delivery evidence trailing data: %w", err)
	}
	if err := Validate(b); err != nil {
		return b, err
	}
	want, _ := CanonicalJSON(b)
	if !bytes.Equal(data, want) {
		return b, errors.New("delivery evidence is not canonical JSON")
	}
	return b, nil
}

func Validate(b Bundle) error {
	switch b.Schema {
	case SchemaV1:
		if b.Authority.Kind != "" || b.RiskProfile != "" {
			return errors.New("schema v1 cannot contain v2 delivery authority or risk profile")
		}
		if b.Spec.Number <= 0 || blank(b.Spec.NodeID) {
			return errors.New("spec number and GitHub node ID are required")
		}
		if b.Spec.Number == b.Issue.Number || b.Spec.NodeID == b.Issue.NodeID {
			return errors.New("issue and spec identities must be distinct")
		}
		if !digest(b.Authority.IssueSHA256) || !digest(b.Authority.SpecSHA256) {
			return errors.New("canonical issue and spec SHA-256 digests are required")
		}
	case SchemaV2:
		if !digest(b.Authority.IssueSHA256) {
			return errors.New("canonical issue SHA-256 digest is required")
		}
		switch b.Authority.Kind {
		case AuthoritySelfContainedIssue:
			if b.Spec != (SpecIdentity{}) || b.Authority.SpecSHA256 != "" {
				return errors.New("self-contained issue authority must not contain a specification identity or digest")
			}
		case AuthorityIssueWithSpecification:
			if b.Spec.Number <= 0 || blank(b.Spec.NodeID) {
				return errors.New("spec number and GitHub node ID are required")
			}
			if b.Spec.Number == b.Issue.Number || b.Spec.NodeID == b.Issue.NodeID {
				return errors.New("issue and spec identities must be distinct")
			}
			if !digest(b.Authority.SpecSHA256) {
				return errors.New("canonical spec SHA-256 digest is required")
			}
		default:
			return fmt.Errorf("invalid delivery authority kind %q", b.Authority.Kind)
		}
		switch b.RiskProfile {
		case RiskLow, RiskStandard, RiskHigh:
		default:
			return fmt.Errorf("invalid delivery risk profile %q", b.RiskProfile)
		}
	default:
		return fmt.Errorf("unsupported delivery evidence schema %q", b.Schema)
	}
	if !slug(b.Repository.Owner) || !slug(b.Repository.Name) || blank(b.Repository.NodeID) {
		return errors.New("repository owner, name, and GitHub node ID are required")
	}
	if b.Issue.Number <= 0 || blank(b.Issue.NodeID) {
		return errors.New("issue number and GitHub node ID are required")
	}
	if b.Iterations == nil {
		return errors.New("iterations must be an explicit array")
	}
	if !gitSHA(b.StartingBaseSHA) {
		return errors.New("starting base must be a full Git SHA")
	}
	if err := uniqueStrings("labels", b.Authority.Labels, true); err != nil {
		return err
	}
	for _, label := range b.Authority.Labels {
		if err := safeText("label", label); err != nil {
			return err
		}
	}
	if err := uniqueStrings("acceptance criteria", b.Authority.AcceptanceCriteria, true); err != nil {
		return err
	}
	for _, criterion := range b.Authority.AcceptanceCriteria {
		if err := safeText("acceptance criterion identity", criterion); err != nil {
			return err
		}
	}
	if len(b.Authority.AcceptanceCriteria) == 0 {
		return errors.New("acceptance criteria must not be empty")
	}
	seen := map[string]string{}
	for _, d := range b.Authority.DependencyDisposition {
		if err := safeText("dependency identity", d.Identity); err != nil {
			return err
		}
		if blank(string(d.Disposition)) {
			return errors.New("dependency identity and disposition are required")
		}
		if d.Disposition != DependencyBlocking && d.Disposition != DependencySatisfied {
			return fmt.Errorf("dependency %q has invalid disposition", d.Identity)
		}
		if _, ok := seen[d.Identity]; ok {
			return fmt.Errorf("duplicate dependency identity %q", d.Identity)
		}
		seen[d.Identity] = string(d.Disposition)
	}
	check := func(name, id, requirement, link string) error {
		if err := safeText(name+" identity", id); err != nil {
			return err
		}
		if err := safeText(name+" requirement", requirement); err != nil {
			return err
		}
		if err := safeText(name+" evidence link", link); err != nil {
			return err
		}
		if blank(id) || blank(requirement) || blank(link) {
			return fmt.Errorf("scope ledger %s requires identity, requirement, and evidence link", name)
		}
		if old, ok := seen[id]; ok {
			return fmt.Errorf("contradictory ledger identity %q appears in %s and %s", id, old, name)
		}
		seen[id] = name
		return nil
	}
	if b.Scope.OwnedNow == nil || b.Scope.Deferred == nil || b.Scope.Forbidden == nil || b.Scope.Prerequisites == nil {
		return errors.New("all scope ledger categories must be explicit")
	}
	for _, e := range b.Scope.OwnedNow {
		if err := check("owned_now", e.Identity, e.Requirement, e.EvidenceLink); err != nil {
			return err
		}
	}
	for _, e := range b.Scope.Forbidden {
		if err := check("forbidden", e.Identity, e.Requirement, e.EvidenceLink); err != nil {
			return err
		}
	}
	for _, e := range b.Scope.Deferred {
		if err := check("deferred", e.Identity, e.Requirement, e.EvidenceLink); err != nil {
			return err
		}
		if err := safeText("deferred owner", e.Owner); err != nil {
			return err
		}
		if blank(e.Owner) {
			return fmt.Errorf("deferred identity %q requires concrete owner", e.Identity)
		}
	}
	for _, e := range b.Scope.Prerequisites {
		if err := check("prerequisites", e.Identity, e.Requirement, e.EvidenceLink); err != nil {
			return err
		}
		if err := safeText("prerequisite disposition", e.Disposition); err != nil {
			return err
		}
		if err := safeText("prerequisite exception boundary", e.ExceptionBoundary); err != nil {
			return err
		}
		if blank(e.Disposition) || blank(e.ExceptionBoundary) {
			return fmt.Errorf("prerequisite identity %q requires disposition and exception boundary", e.Identity)
		}
	}
	if b.AcceptanceMatrix == nil {
		return errors.New("acceptance matrix must be explicit")
	}
	rows := map[string]bool{}
	for _, r := range b.AcceptanceMatrix {
		for name, value := range map[string]string{"identity": r.Identity, "criterion": r.Criterion, "owning seam": r.OwningSeam, "positive": r.PositiveEvidence, "negative": r.NegativeEvidence, "failure": r.FailureEvidence, "mutation": r.MutationEvidence, "compatibility": r.CompatibilityEvidence, "preservation": r.PreservationEvidence, "migration": r.MigrationEvidence} {
			if err := safeText("acceptance "+name, value); err != nil {
				return err
			}
		}
		if blank(r.Identity) || blank(r.Criterion) || blank(r.OwningSeam) || blank(r.PositiveEvidence) || blank(r.NegativeEvidence) || blank(r.FailureEvidence) || blank(r.MutationEvidence) || blank(r.CompatibilityEvidence) || blank(r.PreservationEvidence) || blank(r.MigrationEvidence) {
			return errors.New("acceptance row requires criterion, seam, positive, negative, failure, mutation, compatibility, preservation, and migration evidence")
		}
		if r.State != AcceptancePlanned && r.State != AcceptanceImplemented && r.State != AcceptanceProved {
			return fmt.Errorf("acceptance row %q has invalid state", r.Identity)
		}
		if rows[r.Identity] {
			return fmt.Errorf("duplicate acceptance identity %q", r.Identity)
		}
		rows[r.Identity] = true
	}
	for _, id := range b.Authority.AcceptanceCriteria {
		if !rows[id] {
			return fmt.Errorf("missing acceptance matrix row %s", id)
		}
	}
	if len(rows) != len(b.Authority.AcceptanceCriteria) {
		return errors.New("acceptance matrix contains foreign identity")
	}
	seen = map[string]string{}
	previous := b.StartingBaseSHA
	for i, it := range b.Iterations {
		if err := safeText("iteration identity", it.Identity); err != nil {
			return err
		}
		if it.Sequence != i+1 {
			return errors.New("iteration sequences must be contiguous from 1")
		}
		if blank(it.Identity) || !gitSHA(it.BaseSHA) || !gitSHA(it.HeadSHA) || !digest(it.EvidenceSHA256) {
			return errors.New("iteration identity, base SHA, head SHA, and evidence digest are required")
		}
		if _, ok := seen[it.Identity]; ok {
			return fmt.Errorf("duplicate iteration identity %q", it.Identity)
		}
		seen[it.Identity] = "iteration"
		if it.BaseSHA != previous {
			return fmt.Errorf("iteration %q is disconnected", it.Identity)
		}
		previous = it.HeadSHA
	}
	if err := validateReviews(b); err != nil {
		return err
	}
	if err := validateValidationEvidence(b); err != nil {
		return err
	}
	return nil
}

// marshalBundle preserves the v1 wire shape while allowing a self-contained
// v2 authority to omit specification evidence entirely.
func marshalBundle(bundle Bundle) ([]byte, error) {
	if bundle.Schema != SchemaV2 || bundle.Authority.Kind != AuthoritySelfContainedIssue {
		return json.Marshal(bundle)
	}
	type selfContainedV2 struct {
		Schema             string                      `json:"schema"`
		Repository         RepositoryIdentity          `json:"repository"`
		Issue              IssueIdentity               `json:"issue"`
		Authority          Authority                   `json:"authority"`
		RiskProfile        DeliveryRiskProfile         `json:"risk_profile"`
		Scope              ScopeLedger                 `json:"scope"`
		AcceptanceMatrix   []AcceptanceRow             `json:"acceptance_matrix"`
		StartingBaseSHA    string                      `json:"starting_base_sha"`
		Iterations         []Iteration                 `json:"iterations"`
		ReviewReceipts     []ReviewReceipt             `json:"review_receipts"`
		Adjudications      []Adjudication              `json:"adjudications"`
		ValidationReceipts []ValidationReceipt         `json:"validation_receipts,omitempty"`
		FocusedValidation  []FocusedValidationEvidence `json:"focused_validation,omitempty"`
	}
	return json.Marshal(selfContainedV2{
		Schema: bundle.Schema, Repository: bundle.Repository, Issue: bundle.Issue,
		Authority: bundle.Authority, RiskProfile: bundle.RiskProfile, Scope: bundle.Scope,
		AcceptanceMatrix: bundle.AcceptanceMatrix, StartingBaseSHA: bundle.StartingBaseSHA,
		Iterations: bundle.Iterations, ReviewReceipts: bundle.ReviewReceipts,
		Adjudications: bundle.Adjudications, ValidationReceipts: bundle.ValidationReceipts,
		FocusedValidation: bundle.FocusedValidation,
	})
}

func canonicalize(b *Bundle) {
	sort.Strings(b.Authority.Labels)
	sort.Slice(b.Authority.DependencyDisposition, func(i, j int) bool {
		return b.Authority.DependencyDisposition[i].Identity < b.Authority.DependencyDisposition[j].Identity
	})
	sort.Strings(b.Authority.AcceptanceCriteria)
	sort.Slice(b.Scope.OwnedNow, func(i, j int) bool { return b.Scope.OwnedNow[i].Identity < b.Scope.OwnedNow[j].Identity })
	sort.Slice(b.Scope.Deferred, func(i, j int) bool { return b.Scope.Deferred[i].Identity < b.Scope.Deferred[j].Identity })
	sort.Slice(b.Scope.Forbidden, func(i, j int) bool { return b.Scope.Forbidden[i].Identity < b.Scope.Forbidden[j].Identity })
	sort.Slice(b.Scope.Prerequisites, func(i, j int) bool { return b.Scope.Prerequisites[i].Identity < b.Scope.Prerequisites[j].Identity })
	sort.Slice(b.AcceptanceMatrix, func(i, j int) bool { return b.AcceptanceMatrix[i].Identity < b.AcceptanceMatrix[j].Identity })
	sort.Slice(b.Iterations, func(i, j int) bool { return b.Iterations[i].Sequence < b.Iterations[j].Sequence })
	sort.Slice(b.ReviewReceipts, func(i, j int) bool {
		if b.ReviewReceipts[i].Iteration == b.ReviewReceipts[j].Iteration {
			return b.ReviewReceipts[i].Axis < b.ReviewReceipts[j].Axis
		}
		return b.ReviewReceipts[i].Iteration < b.ReviewReceipts[j].Iteration
	})
	for i := range b.ReviewReceipts {
		sort.Slice(b.ReviewReceipts[i].Findings, func(j, k int) bool {
			return b.ReviewReceipts[i].Findings[j].ID < b.ReviewReceipts[i].Findings[k].ID
		})
	}
	sort.Slice(b.ValidationReceipts, func(i, j int) bool {
		if b.ValidationReceipts[i].CompletedAt == b.ValidationReceipts[j].CompletedAt {
			return b.ValidationReceipts[i].CommitSHA < b.ValidationReceipts[j].CommitSHA
		}
		return b.ValidationReceipts[i].CompletedAt < b.ValidationReceipts[j].CompletedAt
	})
	sort.Slice(b.FocusedValidation, func(i, j int) bool {
		if b.FocusedValidation[i].CompletedAt == b.FocusedValidation[j].CompletedAt {
			return b.FocusedValidation[i].Identity < b.FocusedValidation[j].Identity
		}
		return b.FocusedValidation[i].CompletedAt < b.FocusedValidation[j].CompletedAt
	})
}
func blank(s string) bool { return strings.TrimSpace(s) == "" }
func slug(s string) bool {
	return !blank(s) && s == strings.TrimSpace(s) && !strings.ContainsAny(s, "/\\")
}
func digest(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, e := hex.DecodeString(s)
	return e == nil && strings.ToLower(s) == s
}
func gitSHA(s string) bool { return len(s) == 40 && digest(s+strings.Repeat("0", 24)) }
func uniqueStrings(name string, values []string, require bool) error {
	if require && values == nil {
		return fmt.Errorf("%s must be explicit", name)
	}
	seen := map[string]bool{}
	for _, v := range values {
		if blank(v) {
			return fmt.Errorf("%s contains empty identity", name)
		}
		if seen[v] {
			return fmt.Errorf("duplicate %s identity %q", name, v)
		}
		seen[v] = true
	}
	return nil
}

func clone[T any](in []T) []T {
	if in == nil {
		return nil
	}
	return append(make([]T, 0, len(in)), in...)
}

var unsafeTextPattern = regexp.MustCompile(`(?i)(-----BEGIN [A-Z ]*PRIVATE KEY-----|\bgh[pousr]_[A-Za-z0-9_]+|\bgithub_pat_[A-Za-z0-9_]+|\bsk-[A-Za-z0-9_-]+|\bbearer\s+\S+|\bauthorization\s*:|\b(secret|password|token)\s*[:=]|\b(HOME|XDG_CONFIG_HOME|GITHUB_TOKEN|UPSTREAM_PAYLOAD)\s*=)`)

func safeText(name, value string) error {
	if blank(value) || len(value) > 4096 || !utf8.ValidString(value) {
		return fmt.Errorf("%s must be bounded UTF-8 text", name)
	}
	for _, r := range value {
		if r == '\n' || r == '\r' || unicode.IsControl(r) {
			return fmt.Errorf("%s must be single-line printable text", name)
		}
	}
	if unsafeTextPattern.MatchString(value) {
		return fmt.Errorf("%s contains forbidden secret or environment material", name)
	}
	return nil
}
