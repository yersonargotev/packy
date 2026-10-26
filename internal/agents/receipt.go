package agents

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

var receiptNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (a *SurfaceAdapter) inspectReceipt(ownership []capabilitypack.ProjectionOwnership) (capabilitypack.SurfaceInspection, error) {
	result := capabilitypack.SurfaceInspection{}
	seen := map[string]bool{}
	var revision []string
	for _, owner := range ownership {
		root, target, skill, err := a.receiptTarget(owner.ID)
		if err != nil {
			return result, err
		}
		if seen[owner.ID] || owner.Surface != capabilitypack.SurfaceAgents || owner.PackID == "" || owner.Fingerprint == "" || owner.Target != target {
			return result, fmt.Errorf("invalid Agents receipt ownership for %q", owner.ID)
		}
		seen[owner.ID] = true
		if err := localprojection.ValidateReceiptTarget(root, target, skill); err != nil {
			return result, err
		}
		projection, err := a.inspectOwnedProjection(owner.ID)
		if err != nil {
			return result, err
		}
		projection.Goal = capabilitypack.ProjectionPresent
		projection.DesiredFingerprint = owner.Fingerprint
		projection.Action = capabilitypack.ProjectionAction{ID: owner.ID, Kind: projection.Action.Kind, Target: target}
		result.Projections = append(result.Projections, projection)
		revision = append(revision, owner.ID+"="+projection.ObservedFingerprint)
	}
	sort.Strings(revision)
	result.Revision = localprojection.FingerprintBytes([]byte(strings.Join(revision, "\n")))
	bindAdapterProvenance(&result)
	return result, nil
}

func (a *SurfaceAdapter) receiptTarget(id string) (root, target string, skill bool, err error) {
	kind, name, ok := strings.Cut(id, ":")
	if !ok || !receiptNamePattern.MatchString(name) {
		return "", "", false, fmt.Errorf("unsupported or unsafe Agents receipt projection %q", id)
	}
	switch kind {
	case "skill":
		if a.skillsDir != "" {
			return a.skillsDir, filepath.Join(a.skillsDir, name), true, nil
		}
	case "agent":
		if a.rolesDir != "" {
			return a.rolesDir, filepath.Join(a.rolesDir, name+".toml"), false, nil
		}
	}
	return "", "", false, fmt.Errorf("unsupported or unsafe Agents receipt projection %q", id)
}
