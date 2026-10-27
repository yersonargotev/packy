package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestClaudeDiscoveryPluginContractSchema(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "schemas/cli/v1/skill-discovery.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("discovery.json", doc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("discovery.json#/$defs/pluginExports")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`[{"format":"claude-plugin","name":"guide","prerequisites":[],"references":[],"roles":[]}]`, true},
		{`[{"format":"agent-plugins","name":"guide","prerequisites":[],"references":[],"roles":[]}]`, true},
		{`[{"garbage":true}]`, false},
		{`[{"format":"unknown","name":"guide","prerequisites":[],"references":[],"roles":[]}]`, false},
	} {
		instance, err := jsonschema.UnmarshalJSON(bytes.NewBufferString(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(instance); (err == nil) != tc.valid {
			t.Fatalf("%s valid=%v: %v", tc.body, tc.valid, err)
		}
	}
}
