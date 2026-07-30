package issuedelivery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

type compiledAuthority struct {
	hash      string
	evidence  deliveryevidence.Bundle
	pending   *DecisionRequest
	decisions []Decision
	state     State
	reason    string
}

func compileAuthority(
	git GitObservation,
	tracker TrackerObservation,
	prior []Decision,
	offered *Decision,
	declaredProfile deliveryevidence.DeliveryRiskProfile,
) (compiledAuthority, error) {
	if err := validateObservations(git, tracker); err != nil {
		return compiledAuthority{}, err
	}

	criteria := normalizedItems(tracker.Criteria)
	exclusions := normalizedItems(tracker.Exclusions)
	ambiguities := normalizedItems(tracker.Ambiguities)
	dependencies := normalizedDependencies(tracker.Dependencies)
	references := normalizedReferences(tracker.References)
	labels := normalizedStrings(tracker.Labels)

	available := make(map[string]Decision, len(prior))
	for _, decision := range prior {
		available[decision.RequestID] = decision
	}
	applied := make([]Decision, 0, len(prior)+1)
	offeredUsed := false
	for {
		pending := nextDecisionRequest(tracker.Issue.Number, criteria, ambiguities)
		if pending == nil {
			break
		}
		decision, ok := available[pending.ID]
		if !ok && offered != nil && !offeredUsed {
			if offered.RequestID != pending.ID {
				return compiledAuthority{}, &DecisionMismatchError{Expected: pending.ID, Got: offered.RequestID}
			}
			decision, ok, offeredUsed = *offered, true, true
		}
		if !ok {
			return compilePending(
				tracker, labels, criteria, exclusions, ambiguities, dependencies, references, applied, pending,
			)
		}
		if err := applyDecision(&criteria, &exclusions, &ambiguities, pending, decision); err != nil {
			return compiledAuthority{}, err
		}
		applied = append(applied, decision)
	}
	if offered != nil && !offeredUsed {
		return compiledAuthority{}, fmt.Errorf("delivery decision %q was not requested by current authority", offered.RequestID)
	}

	authorityHash, err := authorityDigest(
		tracker, labels, criteria, exclusions, ambiguities, dependencies, references, applied,
	)
	if err != nil {
		return compiledAuthority{}, err
	}
	bundle, blocked, err := compileBundle(
		git, tracker, labels, criteria, exclusions, dependencies, references, authorityHash, declaredProfile,
	)
	if err != nil {
		return compiledAuthority{}, err
	}
	state, reason := StateNeedsReview, "qualification evidence is ready for independent review"
	if blocked {
		state, reason = StateBlocked, "one or more issue dependencies are not satisfied"
	}
	return compiledAuthority{
		hash: authorityHash, evidence: bundle, decisions: applied, state: state, reason: reason,
	}, nil
}

func compilePending(
	tracker TrackerObservation,
	labels []string,
	criteria, exclusions []AuthorityItem,
	ambiguities []AuthorityItem,
	dependencies []DependencyObservation,
	references []ReferenceObservation,
	decisions []Decision,
	pending *DecisionRequest,
) (compiledAuthority, error) {
	hash, err := authorityDigest(
		tracker, labels, criteria, exclusions, ambiguities, dependencies, references, decisions,
	)
	if err != nil {
		return compiledAuthority{}, err
	}
	return compiledAuthority{
		hash: hash, pending: pending, decisions: decisions, state: StateNeedsDecision,
		reason: "qualification requires a typed caller decision",
	}, nil
}

func compileBundle(
	git GitObservation,
	tracker TrackerObservation,
	labels []string,
	criteria, exclusions []AuthorityItem,
	dependencies []DependencyObservation,
	references []ReferenceObservation,
	authorityHash string,
	declaredProfile deliveryevidence.DeliveryRiskProfile,
) (deliveryevidence.Bundle, bool, error) {
	bundle := deliveryevidence.Bundle{
		Schema:      deliveryevidence.SchemaV2,
		Repository:  tracker.Repository,
		Issue:       tracker.Issue,
		RiskProfile: declaredProfile,
		Authority: deliveryevidence.Authority{
			Kind:                  deliveryevidence.AuthoritySelfContainedIssue,
			IssueSHA256:           authorityHash,
			Labels:                labels,
			DependencyDisposition: make([]deliveryevidence.DependencyDisposition, 0, len(dependencies)),
			AcceptanceCriteria:    make([]string, 0, len(criteria)),
		},
		Scope: deliveryevidence.ScopeLedger{
			OwnedNow:      make([]deliveryevidence.LedgerEntry, 0, len(criteria)),
			Deferred:      []deliveryevidence.DeferredEntry{},
			Forbidden:     make([]deliveryevidence.LedgerEntry, 0, len(exclusions)),
			Prerequisites: make([]deliveryevidence.PrerequisiteEntry, 0, len(dependencies)+len(references)),
		},
		AcceptanceMatrix:   make([]deliveryevidence.AcceptanceRow, 0, len(criteria)),
		StartingBaseSHA:    git.StartingBaseSHA,
		Iterations:         []deliveryevidence.Iteration{},
		ReviewReceipts:     []deliveryevidence.ReviewReceipt{},
		Adjudications:      []deliveryevidence.Adjudication{},
		ValidationReceipts: []deliveryevidence.ValidationReceipt{},
		FocusedValidation:  []deliveryevidence.FocusedValidationEvidence{},
	}
	for _, item := range criteria {
		id := stableID("criterion", item.Text)
		bundle.Authority.AcceptanceCriteria = append(bundle.Authority.AcceptanceCriteria, id)
		bundle.Scope.OwnedNow = append(bundle.Scope.OwnedNow, deliveryevidence.LedgerEntry{
			Identity: id, Requirement: item.Text, EvidenceLink: item.EvidenceLink,
		})
		bundle.AcceptanceMatrix = append(bundle.AcceptanceMatrix, deliveryevidence.AcceptanceRow{
			Identity: id, Criterion: item.Text, OwningSeam: "issuedelivery.Advance",
			PositiveEvidence:      "planned: focused positive behavior through Advance",
			NegativeEvidence:      "planned: focused negative behavior through Advance",
			FailureEvidence:       "planned: failure behavior through Advance",
			MutationEvidence:      "planned: persisted run mutation inspection",
			CompatibilityEvidence: "planned: compatibility validation",
			PreservationEvidence:  "planned: prior run byte preservation",
			MigrationEvidence:     "not applicable: new self-contained run",
			State:                 deliveryevidence.AcceptancePlanned,
		})
	}
	for _, item := range exclusions {
		bundle.Scope.Forbidden = append(bundle.Scope.Forbidden, deliveryevidence.LedgerEntry{
			Identity: stableID("forbidden", item.Text), Requirement: item.Text, EvidenceLink: item.EvidenceLink,
		})
	}
	blocked := false
	for _, dependency := range dependencies {
		disposition := deliveryevidence.DependencySatisfied
		if !strings.EqualFold(dependency.State, "closed") {
			disposition = deliveryevidence.DependencyBlocking
			blocked = true
		}
		dispositionID := stableID("dependency", dependency.Identity)
		prerequisiteID := stableID("prerequisite", dependency.Identity)
		bundle.Authority.DependencyDisposition = append(bundle.Authority.DependencyDisposition,
			deliveryevidence.DependencyDisposition{Identity: dispositionID, Disposition: disposition})
		bundle.Scope.Prerequisites = append(bundle.Scope.Prerequisites, deliveryevidence.PrerequisiteEntry{
			Identity: prerequisiteID, Requirement: dependency.Title, EvidenceLink: dependency.URL,
			Disposition:       string(disposition),
			ExceptionBoundary: "delivery cannot complete while this dependency is blocking",
		})
	}
	for _, reference := range references {
		bundle.Scope.Prerequisites = append(bundle.Scope.Prerequisites, deliveryevidence.PrerequisiteEntry{
			Identity: stableID("reference", reference.Identity), Requirement: reference.Identity,
			EvidenceLink: reference.URL, Disposition: "reference",
			ExceptionBoundary: "informational authority reference; it does not expand delivery scope",
		})
	}
	canonical, err := deliveryevidence.CanonicalJSON(bundle)
	if err != nil {
		return deliveryevidence.Bundle{}, false, err
	}
	bundle, err = deliveryevidence.Decode(canonical)
	return bundle, blocked, err
}

func validateObservations(git GitObservation, tracker TrackerObservation) error {
	if git.CommonDir == "" || git.Owner == "" || git.Name == "" || git.StartingBaseSHA == "" {
		return fmt.Errorf("Git observation is incomplete")
	}
	if !git.WorkspaceClean {
		return fmt.Errorf("issue delivery requires a clean workspace")
	}
	if !fullGitSHAPattern.MatchString(git.StartingBaseSHA) ||
		!fullGitSHAPattern.MatchString(git.HeadSHA) ||
		!fullGitSHAPattern.MatchString(git.TreeSHA) {
		return fmt.Errorf("Git observation requires full lowercase commit and tree SHAs")
	}
	if tracker.Repository.Owner != git.Owner || tracker.Repository.Name != git.Name {
		return fmt.Errorf("GitHub repository identity does not match Git origin")
	}
	if tracker.Issue.Number <= 0 || tracker.Issue.NodeID == "" || tracker.Repository.NodeID == "" {
		return fmt.Errorf("GitHub issue observation is incomplete")
	}
	return nil
}

var fullGitSHAPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func applyDecision(criteria, exclusions, ambiguities *[]AuthorityItem, pending *DecisionRequest, decision Decision) error {
	requirement := cleanText(decision.Requirement)
	link := cleanText(decision.EvidenceLink)
	if requirement == "" || link == "" {
		return fmt.Errorf("delivery decision requires a requirement and evidence link")
	}
	item := AuthorityItem{Text: requirement, EvidenceLink: link}
	switch pending.Kind {
	case DecisionSupplyCriterion:
		if decision.Disposition != DecisionOwnedNow {
			return fmt.Errorf("acceptance criterion decisions must be owned-now")
		}
		*criteria = append(*criteria, item)
	case DecisionClassifyAuthorityItem:
		switch decision.Disposition {
		case DecisionOwnedNow:
			*criteria = append(*criteria, item)
		case DecisionForbidden:
			*exclusions = append(*exclusions, item)
		default:
			return fmt.Errorf("invalid delivery decision disposition %q", decision.Disposition)
		}
		*ambiguities = (*ambiguities)[1:]
	}
	return nil
}

func decisionRequest(kind DecisionKind, item AuthorityItem) *DecisionRequest {
	evidence := cleanText(item.Text) + "\x00" + cleanText(item.EvidenceLink)
	options := []DecisionDisposition{DecisionOwnedNow}
	prompt := "Supply an explicit acceptance criterion before delivery can continue."
	if kind == DecisionClassifyAuthorityItem {
		options = append(options, DecisionForbidden)
		prompt = "Classify this authority evidence before delivery can continue."
	}
	return &DecisionRequest{
		ID: stableID("decision:"+string(kind), evidence), Kind: kind,
		Prompt:   prompt,
		Evidence: cleanText(item.Text),
		Options:  options,
	}
}

func nextDecisionRequest(issue int, criteria, ambiguities []AuthorityItem) *DecisionRequest {
	if len(ambiguities) > 0 {
		return decisionRequest(DecisionClassifyAuthorityItem, ambiguities[0])
	}
	if len(criteria) == 0 {
		return decisionRequest(DecisionSupplyCriterion, AuthorityItem{
			Text:         "The issue does not state an explicit acceptance criterion.",
			EvidenceLink: fmt.Sprintf("issue#%d:missing-acceptance-criterion", issue),
		})
	}
	return nil
}

func authorityDigest(
	tracker TrackerObservation,
	labels []string,
	criteria, exclusions []AuthorityItem,
	ambiguities []AuthorityItem,
	dependencies []DependencyObservation,
	references []ReferenceObservation,
	decisions []Decision,
) (string, error) {
	facts := struct {
		Repository   deliveryevidence.RepositoryIdentity `json:"repository"`
		Issue        deliveryevidence.IssueIdentity      `json:"issue"`
		Title        string                              `json:"title"`
		Body         string                              `json:"body"`
		State        string                              `json:"state"`
		Labels       []string                            `json:"labels"`
		Criteria     []AuthorityItem                     `json:"criteria"`
		Exclusions   []AuthorityItem                     `json:"exclusions"`
		Ambiguities  []AuthorityItem                     `json:"ambiguities"`
		Dependencies []DependencyObservation             `json:"dependencies"`
		References   []ReferenceObservation              `json:"references"`
		Decisions    []Decision                          `json:"decisions"`
	}{
		tracker.Repository, tracker.Issue, cleanText(tracker.Title), cleanText(tracker.Body),
		strings.ToLower(cleanText(tracker.State)), labels, criteria, exclusions, ambiguities,
		dependencies, references, decisions,
	}
	data, err := json.Marshal(facts)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func stableID(kind, value string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + strings.ToLower(cleanText(value))))
	return kind + "-" + hex.EncodeToString(sum[:8])
}

func cleanText(value string) string { return strings.Join(strings.Fields(value), " ") }

func normalizedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = cleanText(value); value != "" {
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

func normalizedItems(values []AuthorityItem) []AuthorityItem {
	out := make([]AuthorityItem, 0, len(values))
	for _, value := range values {
		value.Text, value.EvidenceLink = cleanText(value.Text), cleanText(value.EvidenceLink)
		if value.Text != "" && value.EvidenceLink != "" {
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return stableID("item", out[i].Text) < stableID("item", out[j].Text)
	})
	return out
}

func normalizedDependencies(values []DependencyObservation) []DependencyObservation {
	out := append([]DependencyObservation(nil), values...)
	for i := range out {
		out[i].Identity, out[i].Title, out[i].State, out[i].URL =
			cleanText(out[i].Identity), cleanText(out[i].Title), cleanText(out[i].State), cleanText(out[i].URL)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Identity) < strings.ToLower(out[j].Identity) })
	return out
}

func normalizedReferences(values []ReferenceObservation) []ReferenceObservation {
	out := append([]ReferenceObservation(nil), values...)
	for i := range out {
		out[i].Identity, out[i].URL = cleanText(out[i].Identity), cleanText(out[i].URL)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Identity) < strings.ToLower(out[j].Identity) })
	return out
}
