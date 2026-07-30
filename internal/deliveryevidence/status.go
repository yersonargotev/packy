package deliveryevidence

import (
	"fmt"
)

func RenderStatus(bundle Bundle) (string, error) {
	if err := Validate(bundle); err != nil {
		return "", err
	}
	d, _ := Digest(bundle)
	states := map[string]int{"planned": 0, "implemented": 0, "proved": 0}
	for _, r := range bundle.AcceptanceMatrix {
		states[string(r.State)]++
	}
	if bundle.Schema == SchemaV2 {
		authority := fmt.Sprintf("Self-contained issue: issue %s", bundle.Authority.IssueSHA256)
		if bundle.Authority.Kind == AuthorityIssueWithSpecification {
			authority = fmt.Sprintf("Issue with specification: issue %s spec #%d (%s) %s", bundle.Authority.IssueSHA256, bundle.Spec.Number, bundle.Spec.NodeID, bundle.Authority.SpecSHA256)
		}
		return fmt.Sprintf("Issue delivery evidence\nSchema: %s\nRepository: %s/%s (%s)\nIssue: #%d (%s)\nDelivery authority: %s (%s)\nRisk profile: %s\nScope: owned_now=%d deferred=%d forbidden=%d prerequisites=%d\nAcceptance: planned=%d implemented=%d proved=%d\nStarting base: %s\nIterations: %d\nBundle SHA-256: %s\n", bundle.Schema, bundle.Repository.Owner, bundle.Repository.Name, bundle.Repository.NodeID, bundle.Issue.Number, bundle.Issue.NodeID, authority, bundle.Authority.Kind, bundle.RiskProfile, len(bundle.Scope.OwnedNow), len(bundle.Scope.Deferred), len(bundle.Scope.Forbidden), len(bundle.Scope.Prerequisites), states["planned"], states["implemented"], states["proved"], bundle.StartingBaseSHA, len(bundle.Iterations), d), nil
	}
	return fmt.Sprintf("Issue delivery evidence\nRepository: %s/%s (%s)\nIssue: #%d (%s)\nSpec: #%d (%s)\nAuthority: issue %s spec %s\nScope: owned_now=%d deferred=%d forbidden=%d prerequisites=%d\nAcceptance: planned=%d implemented=%d proved=%d\nStarting base: %s\nIterations: %d\nBundle SHA-256: %s\n", bundle.Repository.Owner, bundle.Repository.Name, bundle.Repository.NodeID, bundle.Issue.Number, bundle.Issue.NodeID, bundle.Spec.Number, bundle.Spec.NodeID, bundle.Authority.IssueSHA256, bundle.Authority.SpecSHA256, len(bundle.Scope.OwnedNow), len(bundle.Scope.Deferred), len(bundle.Scope.Forbidden), len(bundle.Scope.Prerequisites), states["planned"], states["implemented"], states["proved"], bundle.StartingBaseSHA, len(bundle.Iterations), d), nil
}
