package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yersonargotev/packy/internal/addyacceptance"
)

type acceptanceRowsFlag []string

func (f *acceptanceRowsFlag) String() string { return strings.Join(*f, ",") }
func (f *acceptanceRowsFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func writeAcceptanceReport(output, repository, commit, workflowDigest, runID string, encodedRows []string) error {
	rows := make([]addyacceptance.AcceptanceRunRow, 0, len(encodedRows))
	for _, encoded := range encodedRows {
		fields := strings.Split(encoded, "\t")
		if len(fields) != 5 {
			return errors.New("acceptance row must contain five tab-separated fields")
		}
		rows = append(rows, addyacceptance.AcceptanceRunRow{
			ID: fields[0], Package: fields[1], OwningTest: fields[2], Result: fields[3], EvidenceSHA256: fields[4],
		})
	}
	context := addyacceptance.PromotionValidationContext{
		PromotionChange: true, Repository: repository, PullRequest: 1,
		BaseSHA: commit, HeadSHA: commit, EvaluatedMergeSHA: commit,
		Workflow: ".github/workflows/ci.yml", WorkflowDigest: workflowDigest,
		MatrixVersion: addyacceptance.PromotionMatrixVersion, RunID: runID,
	}
	report := addyacceptance.NewAcceptanceRunReport(repository, commit, workflowDigest, runID, rows)
	data, err := report.CanonicalJSON(context)
	if err != nil {
		return fmt.Errorf("validate acceptance report: %w", err)
	}
	return writeExclusive(output, data)
}
