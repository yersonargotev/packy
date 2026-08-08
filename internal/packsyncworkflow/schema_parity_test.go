package packsyncworkflow

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestV230InitialAdmissionSchemasMatchRuntimeArtifacts(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	root := filepath.Join("..", "..", "schemas", "pack-source", "v2.3.0")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource(v230SchemaID(entry.Name()), document); err != nil {
			t.Fatal(err)
		}
	}

	sha := strings.Repeat("a", 64)
	fullSHA := strings.Repeat("b", 40)
	provenance := ArtifactProvenance{SourceLockSHA256: sha, LockSetSHA256: sha, ConfigSHA256: sha, ManifestsSHA256: sha}
	identity := InitialAdmissionArtifactIdentity{
		PackID: "orchestrate", ProposedVersion: "1.0.0", ProposedManifestSHA256: sha,
		LegalEvidenceReference: "docs/evidence.json", LegalEvidenceSHA256: sha, ResultBundleSHA256: sha,
	}
	artifacts := []struct {
		schema string
		value  any
	}{
		{"pack-source-validation.schema.json", ValidationArtifact{SchemaVersion: 2, SourceID: "orchestrate-source", PlanID: "pack-sync-plan", BaseSHA: fullSHA, CandidateSHA: fullSHA, ArtifactProvenance: provenance, InitialAdmissionArtifactIdentity: identity, ResultTreeSHA: fullSHA, PackySuite: true, Apply: true}},
		{"pack-source-operational-artifact.schema.json", FailureArtifact{SchemaVersion: 2, State: "blocked", SourceID: "orchestrate-source", PlanID: "pack-sync-plan", BaseSHA: fullSHA, CandidateSHA: fullSHA, ArtifactProvenance: provenance, InitialAdmissionArtifactIdentity: identity, Blockers: []string{"blocked"}, Recovery: []string{"retry"}}},
		{"pack-source-publication.schema.json", PublicationArtifact{SchemaVersion: 2, SourceID: "orchestrate-source", PlanID: "pack-sync-plan", BaseSHA: fullSHA, CandidateSHA: fullSHA, ArtifactProvenance: provenance, InitialAdmissionArtifactIdentity: identity, ResultTreeSHA: fullSHA, HeadSHA: fullSHA, ProvenanceSHA256: sha, BranchName: "sync/orchestrate-source", PRNumber: 1, PRStateSHA256: sha, ManagedTitle: "sync(orchestrate-source)", ManagedMetadataHash: sha, Validation: CompletedValidationGates(), DecisionReady: true, ManualMergeRequired: true, InvalidationConditions: DecisionReadyInvalidationConditions()}},
	}
	for _, artifact := range artifacts {
		t.Run(artifact.schema, func(t *testing.T) {
			switch value := artifact.value.(type) {
			case ValidationArtifact:
				err = value.Validate()
			case FailureArtifact:
				_, err = value.CanonicalJSON()
			case PublicationArtifact:
				err = value.Validate()
			}
			if err != nil {
				t.Fatalf("runtime producer rejected artifact: %v", err)
			}
			raw, err := json.Marshal(artifact.value)
			if err != nil {
				t.Fatal(err)
			}
			var produced any
			if err := json.Unmarshal(raw, &produced); err != nil {
				t.Fatal(err)
			}
			schema, err := compiler.Compile(v230SchemaID(artifact.schema))
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(produced); err != nil {
				t.Fatalf("runtime artifact disagrees with schema: %v\n%s", err, raw)
			}
		})
	}
}

func v230SchemaID(name string) string {
	return "https://yersonargotev.github.io/packy/schemas/pack-source/v2.3.0/" + name
}
