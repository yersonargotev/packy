package capabilitypack

import "testing"

func TestClaudePluginSkillMetadata(t *testing.T) {
	for _, test := range []struct {
		name, fields string
		valid        bool
	}{
		{"compatible", "context: fork\nbackground: false\neffort: high\narguments: [issue]\n", true},
		{"invalid context", "context: typo\n", false},
		{"background without fork", "background: true\n", false},
		{"invalid effort", "effort: extreme\n", false},
		{"invalid list", "arguments: [1]\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			content := []byte("---\nname: guide\ndescription: Reviewed guide\n" + test.fields + "---\nInstructions\n")
			if err := validatePluginSkill(content, "guide", ClaudePluginFormat); (err == nil) != test.valid {
				t.Fatalf("valid=%v: %v", test.valid, err)
			}
		})
	}
}
