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
		case "lifecycle":
			if len(parts) != 2 || owner.Target != a.layout.SettingsFile {
				return result, fmt.Errorf("Claude receipt hook target changed")
			}
			if err = localprojection.ValidateReceiptTarget(a.layout.ConfigDir, owner.Target, false); err != nil {
				return result, err
			}
			settings := ObserveSettings(owner.Target, nil)
			if settings.Err != nil {
				return result, settings.Err
			}
			candidates := []string{}
			for _, entry := range observedHookEntries(settings) {
				candidates = append(candidates, HookOwnershipFingerprint(entry.event, entry.fingerprint))
			}
			p.ObservedFingerprint, p.Exists, err = receiptContributionFingerprint(candidates, owner.Fingerprint)
		case "mcp_server":
			if len(parts) != 2 || owner.Target != parts[1] {
				return result, fmt.Errorf("Claude receipt MCP target changed")
			}
			if err = localprojection.ValidateReceiptTarget(a.layout.Home, a.layout.UserMCPFile, false); err != nil {
				return result, err
			}
			observed := ObserveUserMCP(a.layout.UserMCPFile, owner.Target)
			p.Exists, p.ObservedFingerprint, err = observed.Present, observed.DefinitionFingerprint, observed.Err
		case "skill", "command", "agent", "asset":
			root, allowLink := a.layout.SkillsDir, false
			switch parts[0] {
			case "skill":
				if len(parts) != 2 || owner.Target != filepath.Join(root, parts[1]) {
					return result, fmt.Errorf("Claude receipt skill target changed")
				}
				allowLink = true
			case "command":
				if len(parts) != 2 || (owner.Target != filepath.Join(root, parts[1], "SKILL.md") && owner.Target != filepath.Join(root, parts[1])) {
					return result, fmt.Errorf("Claude receipt command target changed")
				}
				allowLink = owner.Target == filepath.Join(root, parts[1])
			case "agent":
				root = a.layout.AgentsDir
				if len(parts) != 2 || owner.Target != filepath.Join(root, parts[1]+".md") {
					return result, fmt.Errorf("Claude receipt agent target changed")
				}
			case "asset":
				if len(parts) != 5 || parts[1] != "command" || owner.Target != filepath.Join(root, parts[2], parts[4]) {
					return result, fmt.Errorf("Claude receipt asset target changed")
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
func receiptContributionFingerprint(candidates []string, expected string) (string, bool, error) {
	matches := 0
	for _, candidate := range candidates {
		if fingerprintsEqual(candidate, expected) {
			matches++
		}
	}
	if matches > 1 {
		return "", true, fmt.Errorf("Claude receipt contribution is ambiguous")
	}
	if matches == 1 {
		return expected, true, nil
	}
	if len(candidates) == 0 {
		return "missing", false, nil
	}
	sort.Strings(candidates)
	return "unmatched:" + Fingerprint([]byte(strings.Join(candidates, "\n"))), true, nil
}
