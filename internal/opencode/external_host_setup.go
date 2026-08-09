package opencode

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

const openCodeSubagentStatuslinePlugin = "opencode-subagent-statusline"

func (a *SurfaceAdapter) inspectExternalHostSetupContract(setup capabilitypack.ExternalHostSetupCapability) ([]capabilitypack.ObservedProjection, error) {
	contract := setup.OpenCode
	pluginPath := filepath.Join(filepath.Dir(a.configFile), filepath.Clean(contract.PluginFile))
	pluginFingerprint, pluginExists, err := localprojection.FingerprintPath(pluginPath)
	if err != nil {
		return nil, err
	}
	pluginDesired := localprojection.FingerprintBytes([]byte("external-host-setup:opencode:" + setup.Tool + ":plugin"))
	pluginObserved := pluginFingerprint
	if pluginExists {
		pluginObserved = pluginDesired
	}

	tuiPath := filepath.Join(filepath.Dir(a.configFile), filepath.Clean(contract.TUIFile))
	tuiContent, err := readOptionalSurfaceFile(tuiPath)
	if err != nil {
		return nil, err
	}
	tuiMatches, err := topLevelStringArrayCount(tuiContent, tuiPath, "plugin", contract.TUIPlugin)
	if err != nil {
		return nil, err
	}
	tuiExists := tuiMatches > 0
	tuiConfigured := tuiMatches == 1
	tuiDesired := localprojection.FingerprintBytes([]byte("external-host-setup:opencode:" + setup.Tool + ":tui-plugin"))
	tuiExact := "missing"
	tuiObserved := "missing"
	if tuiExists {
		tuiExact = localprojection.FingerprintBytes([]byte(fmt.Sprintf("%s\ncount=%d", contract.TUIPlugin, tuiMatches)))
	}
	if tuiConfigured {
		tuiObserved = tuiDesired
	}
	return []capabilitypack.ObservedProjection{
		{
			ID: "external_setup:" + setup.Tool + ":opencode:plugin", Exists: pluginExists, ObservedFingerprint: pluginObserved, ExactFingerprint: pluginFingerprint,
			DesiredFingerprint: pluginDesired, ExternallyManaged: true,
			Action: capabilitypack.ProjectionAction{ID: "external_setup:" + setup.Tool + ":opencode:plugin", Kind: capabilitypack.ActionOpenCodeAssetFile, Target: pluginPath, Description: fmt.Sprintf("observe %s-owned OpenCode plugin configuration", setup.Tool)},
		},
		{
			ID: "external_setup:" + setup.Tool + ":opencode:tui-plugin", Exists: tuiExists, ObservedFingerprint: tuiObserved, ExactFingerprint: tuiExact,
			DesiredFingerprint: tuiDesired, ExternallyManaged: true,
			Action: capabilitypack.ProjectionAction{ID: "external_setup:" + setup.Tool + ":opencode:tui-plugin", Kind: capabilitypack.ActionOpenCodeAssetFile, Target: tuiPath, Description: fmt.Sprintf("observe %s-owned OpenCode TUI plugin configuration", setup.Tool)},
		},
	}, nil
}

func topLevelStringArrayCount(content, path, key, value string) (int, error) {
	if content == "" {
		return 0, nil
	}
	config, err := decodeConfig(content, path)
	if err != nil {
		return 0, err
	}
	items, ok := config[key].([]any)
	if !ok {
		return 0, nil
	}
	matches := 0
	for _, item := range items {
		if item == value {
			matches++
		}
	}
	return matches, nil
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
	if matching != 1 {
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
