package codex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/matty/internal/capabilitypack"
)

const (
	instructionStart = "<!-- matty:pack:matty-guidance:start -->"
	instructionEnd   = "<!-- matty:pack:matty-guidance:end -->"
)

type ActivationAdapter struct {
	bundleRoot string
	skillsDir  string
	promptFile string
}

type stagedAction struct {
	action       capabilitypack.ProjectionAction
	temp, backup string
	hadTarget    bool
}

func NewActivationAdapter(bundleRoot, skillsDir, promptFile string) *ActivationAdapter {
	return &ActivationAdapter{bundleRoot: bundleRoot, skillsDir: skillsDir, promptFile: promptFile}
}

func (a *ActivationAdapter) InspectActivation(_ context.Context, pack capabilitypack.Pack) (capabilitypack.ActivationObservation, error) {
	var projections []capabilitypack.ObservedProjection
	var revisionParts []string
	for _, resource := range pack.Resources {
		source := filepath.Join(a.bundleRoot, filepath.Clean(resource.Source))
		switch resource.Kind {
		case "skill":
			desired, err := fingerprintTree(source)
			if err != nil {
				return capabilitypack.ActivationObservation{}, fmt.Errorf("fingerprint skill %q: %w", resource.ID, err)
			}
			target := filepath.Join(a.skillsDir, resource.ID)
			observed, exists, err := fingerprintPath(target)
			if err != nil {
				return capabilitypack.ActivationObservation{}, err
			}
			id := "skill:" + resource.ID
			projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionSkillLink, Source: source, Target: target, Description: fmt.Sprintf("link skill %s at %s", resource.ID, target)}})
			revisionParts = append(revisionParts, id+"="+observed)
		case "instruction":
			content, err := os.ReadFile(source)
			if err != nil {
				return capabilitypack.ActivationObservation{}, fmt.Errorf("read instruction %q: %w", resource.ID, err)
			}
			desiredBlock := instructionStart + "\n" + strings.TrimSpace(string(content)) + "\n" + instructionEnd
			current, err := os.ReadFile(a.promptFile)
			if err != nil && !os.IsNotExist(err) {
				return capabilitypack.ActivationObservation{}, fmt.Errorf("read Codex instructions: %w", err)
			}
			fragment, exists := extractBlock(string(current))
			observed := "missing"
			if exists {
				observed = fingerprintBytes([]byte(fragment))
			}
			desired := fingerprintBytes([]byte(desiredBlock))
			merged := mergeBlock(string(current), desiredBlock)
			id := "instruction:" + resource.ID
			projections = append(projections, capabilitypack.ObservedProjection{ID: id, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired, Action: capabilitypack.ProjectionAction{ID: id, Kind: capabilitypack.ActionInstructionFile, Target: a.promptFile, Content: merged, Description: fmt.Sprintf("write instruction %s in %s", resource.ID, a.promptFile)}})
			revisionParts = append(revisionParts, "prompt="+fingerprintBytes(current))
		}
	}
	sort.Strings(revisionParts)
	return capabilitypack.ActivationObservation{Revision: fingerprintBytes([]byte(strings.Join(revisionParts, "\n"))), Projections: projections}, nil
}

func (a *ActivationAdapter) ApplyProjections(_ context.Context, actions []capabilitypack.ProjectionAction) error {
	items := make([]stagedAction, 0, len(actions))
	cleanup := func() {
		for _, item := range items {
			os.RemoveAll(item.temp)
			os.RemoveAll(item.backup)
		}
	}
	defer cleanup()
	for _, action := range actions {
		if err := os.MkdirAll(filepath.Dir(action.Target), 0o755); err != nil {
			return err
		}
		temp := filepath.Join(filepath.Dir(action.Target), ".matty-stage-"+fingerprintBytes([]byte(action.ID))[:12])
		_ = os.RemoveAll(temp)
		switch action.Kind {
		case capabilitypack.ActionSkillLink:
			if err := os.Symlink(action.Source, temp); err != nil {
				return fmt.Errorf("stage %s: %w", action.ID, err)
			}
		case capabilitypack.ActionInstructionFile:
			if err := os.WriteFile(temp, []byte(action.Content), 0o600); err != nil {
				return fmt.Errorf("stage %s: %w", action.ID, err)
			}
		default:
			return fmt.Errorf("unsupported Codex projection action %q", action.Kind)
		}
		_, err := os.Lstat(action.Target)
		items = append(items, stagedAction{action: action, temp: temp, backup: temp + ".backup", hadTarget: err == nil})
	}
	committed := 0
	for i := range items {
		item := &items[i]
		if item.hadTarget {
			if err := os.Rename(item.action.Target, item.backup); err != nil {
				rollback(items[:committed])
				return err
			}
		}
		if err := os.Rename(item.temp, item.action.Target); err != nil {
			if item.hadTarget {
				_ = os.Rename(item.backup, item.action.Target)
			}
			rollback(items[:committed])
			return err
		}
		committed++
	}
	return nil
}

func rollback(items []stagedAction) {
	for i := len(items) - 1; i >= 0; i-- {
		_ = os.RemoveAll(items[i].action.Target)
		if items[i].hadTarget {
			_ = os.Rename(items[i].backup, items[i].action.Target)
		}
	}
}

func extractBlock(content string) (string, bool) {
	start := strings.Index(content, instructionStart)
	if start < 0 {
		return "", false
	}
	relEnd := strings.Index(content[start:], instructionEnd)
	if relEnd < 0 {
		return "", false
	}
	end := start + relEnd + len(instructionEnd)
	return content[start:end], true
}
func mergeBlock(content, block string) string {
	if existing, ok := extractBlock(content); ok {
		return strings.Replace(content, existing, block, 1)
	}
	trimmed := strings.TrimRight(content, "\n")
	if trimmed == "" {
		return block + "\n"
	}
	return trimmed + "\n\n" + block + "\n"
}
func fingerprintBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func fingerprintPath(path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "missing", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "broken", true, nil
		}
		value, err := fingerprintTree(target)
		return value, true, err
	}
	if info.IsDir() {
		value, err := fingerprintTree(path)
		return value, true, err
	}
	data, err := os.ReadFile(path)
	return fingerprintBytes(data), true, err
}
func fingerprintTree(root string) (string, error) {
	var parts []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parts = append(parts, filepath.ToSlash(rel)+"="+fingerprintBytes(data))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(parts)
	return fingerprintBytes([]byte(strings.Join(parts, "\n"))), nil
}
