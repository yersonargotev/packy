package deliveryevidence

import (
	"fmt"
	"strings"
)

func RenderStatus(bundle Bundle) (string, error) {
	if err := Validate(bundle); err != nil {
		return "", err
	}
	d, _ := Digest(bundle)
	states := map[string]int{"planned": 0, "implemented": 0, "proved": 0}
	for _, r := range bundle.AcceptanceMatrix {
		states[r.State]++
	}
	return fmt.Sprintf("Issue delivery evidence\nRepository: %s/%s (%s)\nIssue: #%d (%s)\nSpec: #%d (%s)\nAuthority: issue %s spec %s\nScope: owned_now=%d deferred=%d forbidden=%d prerequisites=%d\nAcceptance: planned=%d implemented=%d proved=%d\nStarting base: %s\nIterations: %d\nBundle SHA-256: %s\n", bundle.Repository.Owner, bundle.Repository.Name, bundle.Repository.NodeID, bundle.Issue.Number, bundle.Issue.NodeID, bundle.Spec.Number, bundle.Spec.NodeID, bundle.Authority.IssueSHA256, bundle.Authority.SpecSHA256, len(bundle.Scope.OwnedNow), len(bundle.Scope.Deferred), len(bundle.Scope.Forbidden), len(bundle.Scope.Prerequisites), states["planned"], states["implemented"], states["proved"], bundle.StartingBaseSHA, len(bundle.Iterations), d), nil
}

func SameAuthority(a, b Bundle) bool {
	return a.Schema == b.Schema && a.Repository == b.Repository && a.Issue == b.Issue && a.Spec == b.Spec && a.Authority.IssueSHA256 == b.Authority.IssueSHA256 && a.Authority.SpecSHA256 == b.Authority.SpecSHA256 && strings.Join(a.Authority.Labels, "\x00") == strings.Join(b.Authority.Labels, "\x00") && dependenciesEqual(a.Authority.DependencyDisposition, b.Authority.DependencyDisposition)
}
func dependenciesEqual(a, b []DependencyDisposition) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
