package vercelacceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ProofKind string

const (
	ProofPositive ProofKind = "positive"
	ProofNegative ProofKind = "negative"
	ProofOracle   ProofKind = "oracle"
)

type FoundationProof struct {
	Kind ProofKind
	Seam string
}

type FoundationContext struct {
	CandidateSHA string
	RunID        string
	ObservedAt   time.Time
	Now          time.Time
	MaxAge       time.Duration
}

type FoundationDigestRow struct {
	RowID   string
	Digests [3]string
}

type FoundationProofRuns struct {
	Kind   ProofKind
	First  []byte
	Second []byte
}

type FoundationRowProofs struct {
	RowID  string
	Proofs []FoundationProofRuns
}

func (row AcceptanceRow) FoundationProofs() []FoundationProof {
	if row.Source != EvidenceFoundation {
		return nil
	}
	return []FoundationProof{
		{Kind: ProofPositive, Seam: row.EvidenceSeam},
		{Kind: ProofNegative, Seam: row.NegativeSeam},
		{Kind: ProofOracle, Seam: row.OracleSeam},
	}
}

func FoundationProofFilename(rowID string, kind ProofKind, rerun string) (string, error) {
	if !foundationRowID(rowID) || !validProofKind(kind) || (rerun != "first" && rerun != "second") {
		return "", errors.New("invalid foundation proof filename identity")
	}
	return rowID + "." + string(kind) + "." + rerun + ".txt", nil
}

func CanonicalFoundationManifest(ctx FoundationContext, rows []FoundationDigestRow) ([]byte, error) {
	if err := validateFoundationContext(ctx); err != nil {
		return nil, err
	}
	if len(rows) != len(FoundationRows()) {
		return nil, errors.New("foundation manifest requires every foundation row")
	}
	byID := make(map[string][3]string, len(rows))
	for _, row := range rows {
		if !foundationRowID(row.RowID) {
			return nil, fmt.Errorf("unknown foundation row %s", row.RowID)
		}
		if _, duplicate := byID[row.RowID]; duplicate {
			return nil, fmt.Errorf("duplicate foundation row %s", row.RowID)
		}
		for _, value := range row.Digests {
			if !lowerHexDigest(value) {
				return nil, fmt.Errorf("%s has invalid proof digest", row.RowID)
			}
		}
		byID[row.RowID] = row.Digests
	}
	var manifest strings.Builder
	fmt.Fprintf(&manifest, "schema_version\t1\nmatrix_version\t%s\ncandidate_sha\t%s\nfixture_sha256\t%s\nrun_id\t%s\nobserved_at\t%s\n",
		AcceptanceMatrixVersion, ctx.CandidateSHA, ExactArchiveSHA256, ctx.RunID, ctx.ObservedAt.Format(time.RFC3339))
	for _, row := range FoundationRows() {
		digests, ok := byID[row.ID]
		if !ok {
			return nil, fmt.Errorf("%s foundation row is absent", row.ID)
		}
		fmt.Fprintf(&manifest, "row\t%s\t%s\t%s\t%s\n", row.ID, digests[0], digests[1], digests[2])
	}
	return []byte(manifest.String()), nil
}

func ParseFoundationDigestRows(data []byte) ([]FoundationDigestRow, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != len(FoundationRows())*3 {
		return nil, errors.New("foundation proof digests are incomplete")
	}
	byID := make(map[string]FoundationDigestRow, len(FoundationRows()))
	seen := make(map[string]bool, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) != 4 || parts[0] != "proof" {
			return nil, errors.New("malformed foundation proof digest")
		}
		kind := ProofKind(parts[2])
		index, ok := proofIndex(kind)
		key := parts[1] + "\x00" + parts[2]
		if !ok || !foundationRowID(parts[1]) || seen[key] || !lowerHexDigest(parts[3]) {
			return nil, errors.New("invalid or duplicate foundation proof digest")
		}
		seen[key] = true
		row := byID[parts[1]]
		row.RowID = parts[1]
		row.Digests[index] = parts[3]
		byID[parts[1]] = row
	}
	rows := make([]FoundationDigestRow, 0, len(FoundationRows()))
	for _, expected := range FoundationRows() {
		row, ok := byID[expected.ID]
		if !ok {
			return nil, fmt.Errorf("%s foundation proof digests are absent", expected.ID)
		}
		for _, digest := range row.Digests {
			if !lowerHexDigest(digest) {
				return nil, fmt.Errorf("%s foundation proof digests are incomplete", expected.ID)
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func ValidateFoundationEvidence(candidateSHA, runID string, now time.Time, maxAge time.Duration, manifest []byte, artifacts []FoundationRowProofs) ([]RowEvidence, error) {
	ctx, digests, err := parseFoundationManifest(candidateSHA, runID, now, maxAge, manifest)
	if err != nil {
		return nil, err
	}
	byID := make(map[string][]FoundationProofRuns, len(artifacts))
	for _, artifact := range artifacts {
		if _, duplicate := byID[artifact.RowID]; duplicate {
			return nil, fmt.Errorf("duplicate foundation artifact %s", artifact.RowID)
		}
		byID[artifact.RowID] = artifact.Proofs
	}
	var evidence []RowEvidence
	for _, row := range FoundationRows() {
		runs, ok := byID[row.ID]
		if !ok {
			return nil, fmt.Errorf("%s foundation artifact is absent", row.ID)
		}
		proofs := row.FoundationProofs()
		if len(runs) != len(proofs) {
			return nil, fmt.Errorf("%s foundation proof set is incomplete", row.ID)
		}
		var combined []byte
		for index, proof := range proofs {
			run := runs[index]
			if run.Kind != proof.Kind {
				return nil, fmt.Errorf("%s foundation proof order is mixed", row.ID)
			}
			if err := validateFoundationProof(ctx, row, proof, run, digests[row.ID][index]); err != nil {
				return nil, err
			}
			combined = append(combined, run.First...)
			combined = append(combined, 0)
		}
		item := RowEvidence{
			RowID: row.ID, CandidateSHA: ctx.CandidateSHA, FixtureSHA256: ExactArchiveSHA256,
			RunID: ctx.RunID, ObservedAt: ctx.ObservedAt, Passed: true, NegativeTwin: true,
			Deterministic: true, ZeroMutation: true, EvidenceSHA256: rawDigest(combined),
		}
		item.EvidenceFingerprint = FingerprintRowEvidence(item)
		evidence = append(evidence, item)
	}
	return evidence, nil
}

func parseFoundationManifest(candidateSHA, runID string, now time.Time, maxAge time.Duration, data []byte) (FoundationContext, map[string][3]string, error) {
	var ctx FoundationContext
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 6+len(FoundationRows()) {
		return ctx, nil, errors.New("foundation manifest has an invalid row count")
	}
	want := []string{
		"schema_version\t1",
		"matrix_version\t" + AcceptanceMatrixVersion,
		"candidate_sha\t" + candidateSHA,
		"fixture_sha256\t" + ExactArchiveSHA256,
		"run_id\t" + runID,
	}
	for index := range want {
		if lines[index] != want[index] {
			return ctx, nil, errors.New("foundation manifest identity is mixed")
		}
	}
	observed := strings.Split(lines[5], "\t")
	if len(observed) != 2 || observed[0] != "observed_at" {
		return ctx, nil, errors.New("foundation observation time is absent")
	}
	observedAt, err := time.Parse(time.RFC3339, observed[1])
	if err != nil {
		return ctx, nil, errors.New("foundation observation time is invalid")
	}
	ctx = FoundationContext{CandidateSHA: candidateSHA, RunID: runID, ObservedAt: observedAt, Now: now, MaxAge: maxAge}
	if err := validateFoundationContext(ctx); err != nil {
		return ctx, nil, err
	}
	byID := make(map[string][3]string, len(FoundationRows()))
	for _, line := range lines[6:] {
		parts := strings.Split(line, "\t")
		if len(parts) != 5 || parts[0] != "row" || !foundationRowID(parts[1]) {
			return ctx, nil, errors.New("malformed or unknown foundation manifest row")
		}
		if _, duplicate := byID[parts[1]]; duplicate {
			return ctx, nil, fmt.Errorf("duplicate foundation row %s", parts[1])
		}
		digests := [3]string{parts[2], parts[3], parts[4]}
		for _, value := range digests {
			if !lowerHexDigest(value) {
				return ctx, nil, fmt.Errorf("%s has invalid proof digest", parts[1])
			}
		}
		byID[parts[1]] = digests
	}
	for _, row := range FoundationRows() {
		if _, ok := byID[row.ID]; !ok {
			return ctx, nil, fmt.Errorf("%s foundation row is absent", row.ID)
		}
	}
	return ctx, byID, nil
}

func validateFoundationContext(ctx FoundationContext) error {
	decoded, err := hex.DecodeString(ctx.CandidateSHA)
	if err != nil || len(decoded) != 20 || ctx.CandidateSHA != strings.ToLower(ctx.CandidateSHA) || !safeRunID(ctx.RunID) ||
		ctx.ObservedAt.IsZero() || ctx.ObservedAt.Location() != time.UTC ||
		ctx.Now.IsZero() || ctx.MaxAge <= 0 {
		return errors.New("foundation identity is incomplete")
	}
	age := ctx.Now.Sub(ctx.ObservedAt)
	if age < 0 || age > ctx.MaxAge {
		return errors.New("foundation identity is stale")
	}
	return nil
}

func safeRunID(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && !strings.ContainsRune("_.:-", character) {
			return false
		}
	}
	return true
}

func validateFoundationProof(ctx FoundationContext, row AcceptanceRow, proof FoundationProof, run FoundationProofRuns, wantDigest string) error {
	if proof.Seam == "" || !strings.HasPrefix(proof.Seam, "./internal/") {
		return fmt.Errorf("%s has invalid %s seam", row.ID, proof.Kind)
	}
	if string(run.First) != string(run.Second) {
		return fmt.Errorf("%s %s deterministic rerun changed", row.ID, proof.Kind)
	}
	if rawDigest(run.First) != wantDigest {
		return fmt.Errorf("%s %s digest does not match manifest", row.ID, proof.Kind)
	}
	slash := strings.LastIndex(proof.Seam, "/")
	if slash < 0 {
		return fmt.Errorf("%s has invalid %s seam", row.ID, proof.Kind)
	}
	identity := "@identity\t" + ctx.CandidateSHA + "\t" + ctx.RunID + "\t" + ctx.ObservedAt.Format(time.RFC3339) + "\t" + proof.Seam + "\n"
	if !strings.HasPrefix(string(run.First), identity) {
		return fmt.Errorf("%s %s proof identity is mixed", row.ID, proof.Kind)
	}
	test := proof.Seam[slash+1:]
	output := strings.TrimPrefix(string(run.First), identity)
	if strings.Count(output, "=== RUN   "+test+"\n") != 1 ||
		strings.Count(output, "--- PASS: "+test+" (duration)\n") != 1 {
		return fmt.Errorf("%s lacks exact %s RUN/PASS proof", row.ID, proof.Kind)
	}
	return nil
}

func foundationRowID(id string) bool {
	for _, row := range acceptanceRows {
		if row.Source == EvidenceFoundation && row.ID == id {
			return true
		}
	}
	return false
}

func validProofKind(kind ProofKind) bool {
	return kind == ProofPositive || kind == ProofNegative || kind == ProofOracle
}

func proofIndex(kind ProofKind) (int, bool) {
	switch kind {
	case ProofPositive:
		return 0, true
	case ProofNegative:
		return 1, true
	case ProofOracle:
		return 2, true
	default:
		return 0, false
	}
}

func rawDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
