package deliveryevidence

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ReviewAxis string

const (
	ReviewStandards ReviewAxis = "standards"
	ReviewSpec      ReviewAxis = "spec"
)

type FindingSeverity string

const (
	SeverityP0 FindingSeverity = "P0"
	SeverityP1 FindingSeverity = "P1"
	SeverityP2 FindingSeverity = "P2"
	SeverityP3 FindingSeverity = "P3"
)

type FindingAuthority string

const (
	AuthorityDocumentedStandard FindingAuthority = "documented-standard"
	AuthorityJudgmentCallSmell  FindingAuthority = "judgment-call-smell"
	AuthoritySpecRequirement    FindingAuthority = "spec-requirement"
)

type ReviewFinding struct {
	ID        string           `json:"id"`
	Axis      ReviewAxis       `json:"axis"`
	Severity  FindingSeverity  `json:"severity"`
	Authority FindingAuthority `json:"authority"`
	Citation  string           `json:"citation"`
	Location  string           `json:"location"`
	Evidence  string           `json:"evidence"`
}

// ReviewReceipt is reviewer evidence about exactly one declared iteration
// delta. It conveys no approval, scope, waiver, or external mutation authority.
type ReviewReceipt struct {
	IssueNumber int             `json:"issue_number"`
	Iteration   string          `json:"iteration"`
	BaseSHA     string          `json:"base_sha"`
	HeadSHA     string          `json:"head_sha"`
	Axis        ReviewAxis      `json:"axis"`
	Findings    []ReviewFinding `json:"findings"`
}

type FindingDisposition string

const (
	DispositionAccepted                 FindingDisposition = "accepted"
	DispositionRejectedWithEvidence     FindingDisposition = "rejected-with-evidence"
	DispositionScoped                   FindingDisposition = "scoped"
	DispositionSuperseded               FindingDisposition = "superseded"
	DispositionRepairedByLaterIteration FindingDisposition = "repaired-by-later-iteration"
)

// Adjudications are ordered, append-only events. Sequence gives their durable
// order; a later event never removes or rewrites earlier review history.
type Adjudication struct {
	Sequence        int                `json:"sequence"`
	FindingID       string             `json:"finding_id"`
	Disposition     FindingDisposition `json:"disposition"`
	Evidence        string             `json:"evidence"`
	RepairIteration string             `json:"repair_iteration"`
}

type ReviewStatus struct {
	UncoveredCommits   []string `json:"uncovered_commits"`
	UnreviewedDeltas   []string `json:"unreviewed_deltas"`
	UnresolvedFindings []string `json:"unresolved_findings"`
}

func RecordIteration(bundle Bundle, iteration Iteration) (Bundle, error) {
	bundle.Iterations = append(clone(bundle.Iterations), iteration)
	if err := Validate(bundle); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

func RecordReview(bundle Bundle, receipt ReviewReceipt) (Bundle, error) {
	bundle.ReviewReceipts = append(clone(bundle.ReviewReceipts), receipt)
	if err := Validate(bundle); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

func RecordAdjudication(bundle Bundle, adjudication Adjudication) (Bundle, error) {
	bundle.Adjudications = append(clone(bundle.Adjudications), adjudication)
	if err := Validate(bundle); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

func validateReviews(b Bundle) error {
	iterations := make(map[string]Iteration, len(b.Iterations))
	for _, it := range b.Iterations {
		iterations[it.Identity] = it
	}
	paired := make(map[string]map[ReviewAxis]bool, len(b.Iterations))
	findings := make(map[string]ReviewFinding)
	findingIterations := make(map[string]string)
	for _, receipt := range b.ReviewReceipts {
		it, ok := iterations[receipt.Iteration]
		if !ok {
			return fmt.Errorf("review receipt names unknown iteration %q", receipt.Iteration)
		}
		if receipt.IssueNumber != b.Issue.Number {
			return fmt.Errorf("review receipt for iteration %q belongs to another issue", receipt.Iteration)
		}
		if receipt.BaseSHA != it.BaseSHA || receipt.HeadSHA != it.HeadSHA {
			return fmt.Errorf("review receipt for iteration %q does not match its exact delta", receipt.Iteration)
		}
		if receipt.Axis != ReviewStandards && receipt.Axis != ReviewSpec {
			return fmt.Errorf("review receipt for iteration %q has invalid axis", receipt.Iteration)
		}
		if receipt.Findings == nil {
			return fmt.Errorf("review receipt for iteration %q must contain an explicit findings array", receipt.Iteration)
		}
		if paired[receipt.Iteration] == nil {
			paired[receipt.Iteration] = map[ReviewAxis]bool{}
		}
		if paired[receipt.Iteration][receipt.Axis] {
			return fmt.Errorf("duplicate %s review receipt for iteration %q", receipt.Axis, receipt.Iteration)
		}
		paired[receipt.Iteration][receipt.Axis] = true
		for _, finding := range receipt.Findings {
			if err := validateFinding(finding, receipt.Axis); err != nil {
				return err
			}
			if _, duplicate := findings[finding.ID]; duplicate {
				return fmt.Errorf("duplicate finding ID %q", finding.ID)
			}
			findings[finding.ID] = finding
			findingIterations[finding.ID] = receipt.Iteration
		}
	}

	latest := make(map[string]Adjudication)
	for i, event := range b.Adjudications {
		if event.Sequence != i+1 {
			return errors.New("adjudication sequences must be contiguous from 1")
		}
		if _, ok := findings[event.FindingID]; !ok {
			return fmt.Errorf("adjudication names unknown finding %q", event.FindingID)
		}
		if err := safeText("adjudication evidence", event.Evidence); err != nil {
			return err
		}
		switch event.Disposition {
		case DispositionAccepted:
			repair, ok := iterations[event.RepairIteration]
			if !ok {
				return fmt.Errorf("accepted finding %q must name a repair iteration", event.FindingID)
			}
			if repair.Sequence <= iterations[findingIterations[event.FindingID]].Sequence {
				return fmt.Errorf("accepted finding %q must name a later repair iteration", event.FindingID)
			}
		case DispositionRepairedByLaterIteration:
			if _, ok := iterations[event.RepairIteration]; !ok {
				return fmt.Errorf("repaired finding %q must name a repair iteration", event.FindingID)
			}
			prior, accepted := latest[event.FindingID]
			if !accepted || prior.Disposition != DispositionAccepted || prior.RepairIteration != event.RepairIteration {
				return fmt.Errorf("repaired finding %q must follow its accepted repair plan", event.FindingID)
			}
			if !paired[event.RepairIteration][ReviewStandards] || !paired[event.RepairIteration][ReviewSpec] {
				return fmt.Errorf("repair iteration %q lacks paired reviews", event.RepairIteration)
			}
		case DispositionRejectedWithEvidence, DispositionScoped, DispositionSuperseded:
			if event.RepairIteration != "" {
				return fmt.Errorf("%s adjudication for %q forbids a repair iteration", event.Disposition, event.FindingID)
			}
		default:
			return fmt.Errorf("finding %q has invalid disposition", event.FindingID)
		}
		if prior, duplicate := latest[event.FindingID]; duplicate && event.Disposition != DispositionRepairedByLaterIteration {
			return fmt.Errorf("finding %q already has disposition %q", event.FindingID, prior.Disposition)
		}
		latest[event.FindingID] = event
	}
	return nil
}

func validateFinding(f ReviewFinding, axis ReviewAxis) error {
	for name, value := range map[string]string{"ID": f.ID, "citation": f.Citation, "location": f.Location, "evidence": f.Evidence} {
		if err := safeText("finding "+name, value); err != nil {
			return err
		}
	}
	if f.Axis != axis {
		return fmt.Errorf("finding %q axis does not match its receipt", f.ID)
	}
	if f.Severity != SeverityP0 && f.Severity != SeverityP1 && f.Severity != SeverityP2 && f.Severity != SeverityP3 {
		return fmt.Errorf("finding %q has invalid severity", f.ID)
	}
	if axis == ReviewStandards && f.Authority != AuthorityDocumentedStandard && f.Authority != AuthorityJudgmentCallSmell {
		return fmt.Errorf("standards finding %q has invalid authority", f.ID)
	}
	if axis == ReviewSpec && f.Authority != AuthoritySpecRequirement {
		return fmt.Errorf("spec finding %q has invalid authority", f.ID)
	}
	return nil
}

// QueryReviews derives deterministic coverage from the bundle and a linear,
// oldest-to-newest list of commits beginning at StartingBaseSHA.
func QueryReviews(bundle Bundle, head string, orderedCommits []string) (ReviewStatus, error) {
	if err := Validate(bundle); err != nil {
		return ReviewStatus{}, err
	}
	if len(orderedCommits) == 0 || orderedCommits[0] != bundle.StartingBaseSHA || orderedCommits[len(orderedCommits)-1] != head {
		return ReviewStatus{}, errors.New("ordered commits must span the bundle starting base through the requested head")
	}
	index := make(map[string]int, len(orderedCommits))
	for i, commit := range orderedCommits {
		if !gitSHA(commit) {
			return ReviewStatus{}, fmt.Errorf("ordered commit %q is not a full Git SHA", commit)
		}
		if _, duplicate := index[commit]; duplicate {
			return ReviewStatus{}, fmt.Errorf("duplicate ordered commit %q", commit)
		}
		index[commit] = i
	}
	axes := map[string]map[ReviewAxis]bool{}
	for _, receipt := range bundle.ReviewReceipts {
		if axes[receipt.Iteration] == nil {
			axes[receipt.Iteration] = map[ReviewAxis]bool{}
		}
		axes[receipt.Iteration][receipt.Axis] = true
	}
	covered := make(map[string]bool)
	var result ReviewStatus
	for _, it := range bundle.Iterations {
		base, baseOK := index[it.BaseSHA]
		head, headOK := index[it.HeadSHA]
		if !baseOK || !headOK || head <= base {
			return ReviewStatus{}, fmt.Errorf("ordered commits do not contain iteration %q delta", it.Identity)
		}
		if !axes[it.Identity][ReviewStandards] || !axes[it.Identity][ReviewSpec] {
			result.UnreviewedDeltas = append(result.UnreviewedDeltas, it.Identity)
			continue
		}
		for i := base + 1; i <= head; i++ {
			covered[orderedCommits[i]] = true
		}
	}
	for _, commit := range orderedCommits[1:] {
		if !covered[commit] {
			result.UncoveredCommits = append(result.UncoveredCommits, commit)
		}
	}
	resolved := map[string]bool{}
	for _, event := range bundle.Adjudications {
		switch event.Disposition {
		case DispositionRejectedWithEvidence, DispositionScoped, DispositionSuperseded, DispositionRepairedByLaterIteration:
			resolved[event.FindingID] = true
		case DispositionAccepted:
			resolved[event.FindingID] = false
		}
	}
	for _, receipt := range bundle.ReviewReceipts {
		for _, finding := range receipt.Findings {
			if !resolved[finding.ID] {
				result.UnresolvedFindings = append(result.UnresolvedFindings, finding.ID)
			}
		}
	}
	sort.Strings(result.UncoveredCommits)
	sort.Strings(result.UnreviewedDeltas)
	sort.Strings(result.UnresolvedFindings)
	return result, nil
}

func RenderReviewReport(bundle Bundle, head string, orderedCommits []string) (string, error) {
	query, err := QueryReviews(bundle, head, orderedCommits)
	if err != nil {
		return "", err
	}
	canonicalize(&bundle)
	var out strings.Builder
	fmt.Fprintf(&out, "Review evidence\nIssue: #%d\n", bundle.Issue.Number)
	for _, receipt := range bundle.ReviewReceipts {
		fmt.Fprintf(&out, "Receipt: iteration=%s axis=%s base=%s head=%s\n", receipt.Iteration, receipt.Axis, receipt.BaseSHA, receipt.HeadSHA)
		for _, finding := range receipt.Findings {
			fmt.Fprintf(&out, "  Finding: id=%s axis=%s severity=%s authority=%s citation=%s location=%s evidence=%s\n",
				finding.ID, finding.Axis, finding.Severity, finding.Authority, finding.Citation, finding.Location, finding.Evidence)
		}
	}
	for _, event := range bundle.Adjudications {
		fmt.Fprintf(&out, "Adjudication: sequence=%d finding=%s disposition=%s evidence=%s", event.Sequence, event.FindingID, event.Disposition, event.Evidence)
		if event.RepairIteration != "" {
			fmt.Fprintf(&out, " repair_iteration=%s", event.RepairIteration)
		}
		out.WriteByte('\n')
	}
	fmt.Fprintf(&out, "Uncovered commits: %s\nUnreviewed deltas: %s\nUnresolved findings: %s\n",
		renderList(query.UncoveredCommits), renderList(query.UnreviewedDeltas), renderList(query.UnresolvedFindings))
	return out.String(), nil
}

func renderList(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}
