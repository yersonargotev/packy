package governancedrift

import (
	"strings"
	"testing"
	"time"
)

func TestRecoveryContractAdmitsOnlySealedImmutableLatestRelease(t *testing.T) {
	expected, err := NewSanitizedValue([]byte(`{"tag_name":"v0.1.10","draft":false,"prerelease":false,"immutable":true,"published_at":"2026-07-23T04:34:23Z","author":"github-actions[bot]","asset_count":7}`))
	if err != nil {
		t.Fatal(err)
	}
	actual, err := NewSanitizedValue([]byte(`{"tag_name":"v0.1.11","draft":false,"prerelease":false,"immutable":true,"published_at":"2026-07-31T04:34:23Z","author":"github-actions[bot]","asset_count":7}`))
	if err != nil {
		t.Fatal(err)
	}
	contract := Contract{SchemaVersion: ContractSchemaVersion, Controls: []Control{{
		ID: "latest-release", Boundaries: []Boundary{BoundaryPublication}, Expected: expected,
	}}}
	observation := Observation{
		SchemaVersion: ObservationSchemaVersion,
		Identity: EvidenceIdentity{
			Repository: "yersonargotev/packy", Ref: "refs/heads/main",
			CommitSHA: strings.Repeat("a", 40), WorkflowSHA: strings.Repeat("b", 40),
			CollectedAt: time.Date(2026, 7, 31, 4, 35, 0, 0, time.UTC),
		},
		Controls: []ObservedControl{{
			ID: "latest-release", State: ObservationObserved, Actual: actual,
		}},
	}

	effective, err := RecoveryContract(contract, observation, "v0.1.11")
	if err != nil {
		t.Fatal(err)
	}
	if effective.Controls[0].Expected != actual {
		t.Fatalf("effective latest release = %s", effective.Controls[0].Expected)
	}
	if contract.Controls[0].Expected != expected {
		t.Fatal("canonical contract was mutated")
	}

	for name, mutate := range map[string]func(*ObservedControl){
		"wrong tag": func(c *ObservedControl) {
			c.Actual, _ = NewSanitizedValue([]byte(`{"tag_name":"v0.1.12","draft":false,"prerelease":false,"immutable":true,"published_at":"2026-07-31T04:34:23Z","author":"github-actions[bot]","asset_count":7}`))
		},
		"mutable": func(c *ObservedControl) {
			c.Actual, _ = NewSanitizedValue([]byte(`{"tag_name":"v0.1.11","draft":false,"prerelease":false,"immutable":false,"published_at":"2026-07-31T04:34:23Z","author":"github-actions[bot]","asset_count":7}`))
		},
		"wrong actor": func(c *ObservedControl) {
			c.Actual, _ = NewSanitizedValue([]byte(`{"tag_name":"v0.1.11","draft":false,"prerelease":false,"immutable":true,"published_at":"2026-07-31T04:34:23Z","author":"owner","asset_count":7}`))
		},
		"extra asset": func(c *ObservedControl) {
			c.Actual, _ = NewSanitizedValue([]byte(`{"tag_name":"v0.1.11","draft":false,"prerelease":false,"immutable":true,"published_at":"2026-07-31T04:34:23Z","author":"github-actions[bot]","asset_count":8}`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := observation
			changed.Controls = append([]ObservedControl(nil), observation.Controls...)
			mutate(&changed.Controls[0])
			if _, err := RecoveryContract(contract, changed, "v0.1.11"); err == nil {
				t.Fatal("unsafe latest-release transition was admitted")
			}
		})
	}
}
