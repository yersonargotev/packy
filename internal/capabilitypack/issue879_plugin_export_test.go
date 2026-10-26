package capabilitypack

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssue879ClosedExportContract(t *testing.T) {
	valid := `[{"format":"agent-plugins","name":"guide","prerequisites":[],"references":[],"roles":[]}]`
	tests := []struct {
		name  string
		data  string
		valid bool
	}{
		{"reviewed", valid, true},
		{"null", "null", false},
		{"unknown format", strings.Replace(valid, "agent-plugins", "claude-plugin", 1), false},
		{"unknown field", strings.Replace(valid, `"roles":[]`, `"roles":[],"execute":true`, 1), false},
		{"null field", strings.Replace(valid, `"roles":[]`, `"roles":null`, 1), false},
		{"missing prerequisites", strings.Replace(valid, `"prerequisites":[],`, "", 1), false},
		{"escaping source", strings.Replace(valid, `"roles":[]`, `"roles":[],"source":"../foreign"`, 1), false},
		{"bad name", strings.Replace(valid, `"name":"guide"`, `"name":"bad--name"`, 1), false},
		{"escaping reference", strings.Replace(valid, `"references":[]`, `"references":[{"id":"link","file":"../foreign.md","target":"skill:guide","kind":"path"}]`, 1), false},
		{"implicit invocation", strings.Replace(valid, `"references":[]`, `"references":[{"id":"link","file":"SKILL.md","target":"skill:guide","kind":"invocation"}]`, 1), false},
		{"undeclared role", strings.Replace(valid, `"roles":[]`, `"roles":[{"resource":"agent:reviewer","resolution":"external","description":"Needs reviewer"}]`, 1), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var contracts PluginExports
			err := json.Unmarshal([]byte(test.data), &contracts)
			if err == nil {
				err = validatePluginExports(Resource{Kind: "skill", ID: "guide", PluginExports: contracts})
			}
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v: %v", test.valid, err)
			}
		})
	}
}

func TestIssue879StagingFailuresLeaveDestinationUnchanged(t *testing.T) {
	root := t.TempDir()
	snapshot := filepath.Join(root, "snapshot")
	if err := os.Mkdir(snapshot, 0755); err != nil {
		t.Fatal(err)
	}
	for _, existing := range []bool{false, true} {
		name := "new"
		if existing {
			name = "empty"
		}
		output := filepath.Join(root, name)
		if existing {
			if err := os.Mkdir(output, 0700); err != nil {
				t.Fatal(err)
			}
		}
		// The impossible file/child graph forces a real staging write error regardless
		// of map iteration order. The production planner rejects it even earlier.
		_, err := publishPlugin(context.Background(), snapshot, output, map[string]pluginFile{"a": {[]byte("a"), 0644}, "a/child": {[]byte("b"), 0644}})
		if err == nil {
			t.Fatal("staging write error succeeded")
		}
		info, statErr := os.Lstat(output)
		if existing {
			if statErr != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
				t.Fatalf("empty destination altered: %v", statErr)
			}
			entries, err := os.ReadDir(output)
			if err != nil || len(entries) != 0 {
				t.Fatal("partial output published")
			}
		} else if !os.IsNotExist(statErr) {
			t.Fatal("failed new export published output")
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".packy-export-") {
			t.Fatal("staging leaked")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	output := filepath.Join(root, "cancelled")
	if _, err := publishPlugin(ctx, snapshot, output, map[string]pluginFile{"x": {[]byte("a"), 0644}}); err == nil {
		t.Fatal("cancelled export succeeded")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("cancelled export published output")
	}
}

func TestIssue879UnsafeSourcesAndOutput(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "foreign"), []byte("foreign"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../foreign", filepath.Join(source, "linked")); err != nil {
		t.Fatal(err)
	}
	opened, err := os.OpenRoot(source)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if _, err := pluginSourceFiles(context.Background(), opened, "linked", "assets/data", false); err == nil {
		t.Fatal("linked source accepted")
	}
	if _, err := pluginSourceFiles(context.Background(), opened, "../foreign", "assets/data", false); err == nil {
		t.Fatal("escaping source accepted")
	}
	output := filepath.Join(root, "linked-output")
	if err := os.Symlink(source, output); err != nil {
		t.Fatal(err)
	}
	files := map[string]pluginFile{"plugin.json": {[]byte("{}\n"), 0644}}
	if _, err := publishPlugin(context.Background(), source, output, files); err == nil {
		t.Fatal("linked destination accepted")
	}
	if _, err := publishPlugin(context.Background(), source, filepath.Join(source, "output"), files); err == nil {
		t.Fatal("snapshot destination accepted")
	}
}
