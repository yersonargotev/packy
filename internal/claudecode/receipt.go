package claudecode

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

// inspectReceipt observes only persisted projection identities and exact digests.
// It does not reconstruct a historical Pack or invoke Claude.
func (a *SurfaceAdapter) inspectReceipt(owners []capabilitypack.ProjectionOwnership) (capabilitypack.SurfaceInspection, error) {
	result := capabilitypack.SurfaceInspection{}
	revision := []string{}
	for _, owner := range owners {
		parts := strings.Split(owner.ID, ":")
		if len(parts) < 2 {
			return result, fmt.Errorf("invalid Claude receipt projection %q", owner.ID)
		}
		for _, part := range parts {
			if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "/\\\x00") {
				return result, fmt.Errorf("unsafe Claude receipt projection %q", owner.ID)
			}
		}
		p := capabilitypack.ObservedProjection{ID: owner.ID, Goal: capabilitypack.ProjectionPresent, DesiredFingerprint: owner.Fingerprint, ObservedFingerprint: "missing", Action: capabilitypack.ProjectionAction{ID: owner.ID, Target: owner.Target}}
		var err error
		switch parts[0] {
		case "instruction":
			if len(parts) != 2 || owner.Target != a.layout.InstructionsFile {
				return result, fmt.Errorf("Claude receipt instruction target changed")
			}
			if err = localprojection.ValidateReceiptTarget(a.layout.ConfigDir, owner.Target, false); err != nil {
				return result, err
			}
			observation := ObserveInstructions(owner.Target)
			if observation.Err != nil {
				return result, observation.Err
			}
			p.ObservedFingerprint, p.Exists = observation.Contributions["pack:"+owner.PackID+":"+parts[1]]
			if !p.Exists {
				p.ObservedFingerprint = "missing"
			}
		case "skill", "agent":
			root, allowLink := a.layout.SkillsDir, false
			switch parts[0] {
			case "skill":
				if len(parts) != 2 || owner.Target != filepath.Join(root, parts[1]) {
					return result, fmt.Errorf("Claude receipt skill target changed")
				}
				allowLink = true
			case "agent":
				root = a.layout.AgentsDir
				if len(parts) != 2 || owner.Target != filepath.Join(root, parts[1]+".md") {
					return result, fmt.Errorf("Claude receipt agent target changed")
				}
			}
			if err = localprojection.ValidateReceiptTarget(root, owner.Target, allowLink); err != nil {
				return result, err
			}
			info, statErr := os.Lstat(owner.Target)
			if statErr == nil && info.IsDir() {
				p.ObservedFingerprint, p.Exists, err = freshCompositeTreeFingerprint(owner.Target)
			} else {
				p.ObservedFingerprint, p.Exists, err = localprojection.FingerprintPath(owner.Target)
			}
		default:
			return result, fmt.Errorf("unsupported Claude receipt projection %q", owner.ID)
		}
		if err != nil {
			return result, err
		}
		if fingerprintsEqual(p.ObservedFingerprint, owner.Fingerprint) {
			p.ObservedFingerprint = owner.Fingerprint
		}
		result.Projections = append(result.Projections, p)
		revision = append(revision, owner.ID+"="+p.ObservedFingerprint)
	}
	sort.Strings(revision)
	result.Revision = Fingerprint([]byte(strings.Join(revision, "\n")))
	return result, nil
}

// A receipt seals a contribution digest, not its prior semantic body. Exact
// matches remain observable; unmatched contributions conservatively expose drift.
