package opencode

import (
	"bytes"
	"encoding/json"
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
		if seen[owner.ID] || owner.Surface != capabilitypack.SurfaceOpenCode || owner.PackID == "" || owner.Fingerprint == "" || owner.Target != target {
			return result, fmt.Errorf("invalid OpenCode receipt ownership for %q", owner.ID)
		}
		seen[owner.ID] = true
		if err := localprojection.ValidateReceiptTarget(root, target, skill); err != nil {
			return result, err
		}
		configContent := ""
		if target == a.configFile {
			configContent, err = readOptionalSurfaceFile(target)
			if err == nil {
				err = a.validateReceiptConfig(configContent, owner.ID)
			}
		}
		if err != nil {
			return result, err
		}
		projection, ok, err := a.inspectOwnedProjection(owner.ID, configContent)
		if err != nil {
			return result, err
		}
		if !ok {
			return result, fmt.Errorf("unsupported OpenCode receipt projection %q", owner.ID)
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
		return "", "", false, fmt.Errorf("unsupported or unsafe OpenCode receipt projection %q", id)
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
			return filepath.Dir(a.promptFile), a.instructionPath(parts[1]), false, nil
		case "primary_prompt":
			return filepath.Dir(a.promptFile), a.promptFile, false, nil
		case "mcp_server", "opencode-instruction-reference", "opencode-primary-prompt-reference":
			return filepath.Dir(a.configFile), a.configFile, false, nil
		case "agent", "command":
			return filepath.Dir(a.configFile), filepath.Join(filepath.Dir(a.configFile), parts[0]+"s", parts[1]+".md"), false, nil
		}
	}
	if len(parts) == 5 && parts[0] == "asset" {
		switch parts[1] {
		case "skill":
			return a.skillsDir, filepath.Join(a.skillsDir, ".packy-assets", parts[2], parts[4]), false, nil
		case "agent", "command":
			return filepath.Dir(a.configFile), filepath.Join(filepath.Dir(a.configFile), parts[1]+"s", parts[2], parts[4]), false, nil
		}
	}
	return invalid()
}

func (a *SurfaceAdapter) validateReceiptConfig(content, id string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	data, err := jsoncToJSON(content)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := rejectDuplicateReceiptKeys(decoder); err != nil {
		return err
	}
	if strings.HasPrefix(id, "opencode-instruction-reference:") || strings.HasPrefix(id, "opencode-primary-prompt-reference:") {
		instructions, err := configuredInstructions(content, a.configFile)
		if err != nil {
			return err
		}
		matches := 0
		for _, instruction := range instructions {
			if instructionReferencesPath(a.configFile, instruction, a.configReferenceTarget(id)) {
				matches++
			}
		}
		if matches > 1 {
			return fmt.Errorf("ambiguous OpenCode receipt reference %q", id)
		}
	}
	return nil
}

func rejectDuplicateReceiptKeys(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	seen := map[string]bool{}
	for decoder.More() {
		if delimiter == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("ambiguous OpenCode receipt config property %q", key)
			}
			seen[name] = true
		}
		if err := rejectDuplicateReceiptKeys(decoder); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}
