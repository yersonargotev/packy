package opencode

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

const openCodeSubagentStatuslinePlugin = "opencode-subagent-statusline"

func hasEngramOpenCodeSetupResources(pack capabilitypack.Pack) bool {
	hasInstruction, hasMCP := false, false
	for _, resource := range pack.Resources {
		hasInstruction = hasInstruction || resource.Kind == "instruction" && resource.ID == "engram-memory"
		hasMCP = hasMCP || resource.Kind == "mcp_server" && resource.ID == "engram"
	}
	return hasInstruction && hasMCP
}

func (a *SurfaceAdapter) inspectEngramSetupContract() ([]capabilitypack.ObservedProjection, error) {
	pluginPath := filepath.Join(filepath.Dir(a.configFile), "plugins", "engram.ts")
	pluginFingerprint, pluginExists, err := localprojection.FingerprintPath(pluginPath)
	if err != nil {
		return nil, err
	}
	pluginDesired := localprojection.FingerprintBytes([]byte("engram-opencode-contract-v1:plugin"))
	pluginObserved := pluginFingerprint
	if pluginExists {
		pluginObserved = pluginDesired
	}

	tuiPath := filepath.Join(filepath.Dir(a.configFile), "tui.json")
	tuiContent, err := readOptionalSurfaceFile(tuiPath)
	if err != nil {
		return nil, err
	}
	tuiConfigured, err := topLevelStringArrayContains(tuiContent, tuiPath, "plugin", openCodeSubagentStatuslinePlugin)
	if err != nil {
		return nil, err
	}
	tuiDesired := localprojection.FingerprintBytes([]byte("engram-opencode-contract-v1:tui-plugin"))
	tuiExact := "missing"
	tuiObserved := "missing"
	if tuiConfigured {
		tuiExact = localprojection.FingerprintBytes([]byte(openCodeSubagentStatuslinePlugin))
		tuiObserved = tuiDesired
	}
	return []capabilitypack.ObservedProjection{
		{
			ID: "external_setup:engram:opencode:plugin", Exists: pluginExists, ObservedFingerprint: pluginObserved, ExactFingerprint: pluginFingerprint,
			DesiredFingerprint: pluginDesired, ExternallyManaged: true,
			Action: capabilitypack.ProjectionAction{ID: "external_setup:engram:opencode:plugin", Kind: capabilitypack.ActionOpenCodeAssetFile, Target: pluginPath, Description: "observe Engram-owned OpenCode plugin configuration"},
		},
		{
			ID: "external_setup:engram:opencode:tui-plugin", Exists: tuiConfigured, ObservedFingerprint: tuiObserved, ExactFingerprint: tuiExact,
			DesiredFingerprint: tuiDesired, ExternallyManaged: true,
			Action: capabilitypack.ProjectionAction{ID: "external_setup:engram:opencode:tui-plugin", Kind: capabilitypack.ActionOpenCodeAssetFile, Target: tuiPath, Description: "observe Engram-owned OpenCode TUI plugin configuration"},
		},
	}, nil
}

func topLevelStringArrayContains(content, path, key, value string) (bool, error) {
	if content == "" {
		return false, nil
	}
	config, err := decodeConfig(content, path)
	if err != nil {
		return false, err
	}
	items, ok := config[key].([]any)
	if !ok {
		return false, nil
	}
	for _, item := range items {
		if item == value {
			return true, nil
		}
	}
	return false, nil
}

func removeTopLevelStringArrayEntry(content, key, value string) (string, error) {
	property, found, err := findTopLevelProperty(content, key)
	if err != nil || !found {
		return content, err
	}
	elements, _, err := instructionArrayElements(content, property)
	if err != nil {
		return "", err
	}
	remaining := 0
	matching := 0
	for _, element := range elements {
		if element.value == value {
			matching++
		} else {
			remaining++
		}
	}
	if matching == 0 {
		return content, nil
	}
	if remaining == 0 && !arrayValueHasComments(content[property.valueStart:property.valueEnd]) {
		return removeProperty(content, property), nil
	}
	updated := content
	for i := len(elements) - 1; i >= 0; i-- {
		if elements[i].value == value {
			updated = removeArrayElement(updated, elements[i])
		}
	}
	return updated, nil
}

func readExternalSetupFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}
