package codex

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
		if seen[owner.ID] || owner.Surface != capabilitypack.SurfaceCodex || owner.PackID == "" || owner.Fingerprint == "" || owner.Target != target {
			return result, fmt.Errorf("invalid Codex receipt ownership for %q", owner.ID)
		}
		seen[owner.ID] = true
		if err := localprojection.ValidateReceiptTarget(root, target, skill); err != nil {
			return result, err
		}
		promptContent, configContent := "", ""
		if strings.HasPrefix(owner.ID, "instruction:") {
			promptContent, err = readOptionalFile(target)
			if err == nil {
				err = validateReceiptBlock(promptContent, owner.ID, false)
			}
		}
		if strings.HasPrefix(owner.ID, "mcp_server:") {
			configContent, err = readOptionalFile(target)
			if err == nil {
				err = validateReceiptBlock(configContent, owner.ID, true)
			}
		}
		if err != nil {
			return result, err
		}
		projection, ok, err := a.inspectOwnedProjection(owner.ID, promptContent, configContent)
		if err != nil {
			return result, err
		}
		if !ok {
			return result, fmt.Errorf("unsupported Codex receipt projection %q", owner.ID)
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
	parts := strings.Split(id, ":")
	invalid := func() (string, string, bool, error) {
		return "", "", false, fmt.Errorf("unsupported or unsafe Codex receipt projection %q", id)
	}
	if len(parts) != 2 && len(parts) != 5 {
		return invalid()
	}
	for _, name := range parts[:len(parts)-1] {
		if !receiptNamePattern.MatchString(name) && name != "mcp_server" && name != "primary_prompt" {
			return invalid()
		}
	}
	if len(parts) == 2 && !receiptNamePattern.MatchString(parts[1]) {
		return invalid()
	}
	if len(parts) == 5 {
		filename := parts[4]
		if filename == "" || filename == "." || filename == ".." || filepath.Base(filename) != filename || strings.ContainsAny(filename, "\\\r\n\x00") {
			return invalid()
		}
	}
	if len(parts) == 2 {
		switch parts[0] {
		case "skill":
			return a.skillsDir, filepath.Join(a.skillsDir, parts[1]), true, nil
		case "instruction":
			return filepath.Dir(a.promptFile), a.promptFile, false, nil
		case "mcp_server":
			return filepath.Dir(a.configFile), a.configFile, false, nil
		case "agent":
			return filepath.Dir(a.promptFile), filepath.Join(filepath.Dir(a.promptFile), "agents", parts[1]+".toml"), false, nil
		case "workflow":
			return a.skillsDir, filepath.Join(a.skillsDir, parts[1], "SKILL.md"), false, nil
		}
	}
	if len(parts) == 5 && parts[0] == "asset" && parts[1] == "workflow" {
		return a.skillsDir, filepath.Join(a.skillsDir, parts[2], parts[4]), false, nil
	}
	return invalid()
}

func validateReceiptBlock(content, id string, mcp bool) error {
	_, name, _ := strings.Cut(id, ":")
	start, end := instructionMarkers(name)
	if mcp {
		start, end = mcpMarkers(name)
	}
	starts, ends := strings.Count(content, start), strings.Count(content, end)
	if starts == 0 && ends == 0 {
		return nil
	}
	if starts != 1 || ends != 1 || strings.Index(content, start) > strings.Index(content, end) {
		return fmt.Errorf("ambiguous Codex receipt projection %q", id)
	}
	if mcp && strings.Count(content, "[mcp_servers."+name+"]") > 1 {
		return fmt.Errorf("ambiguous Codex MCP receipt projection %q", id)
	}
	return nil
}
