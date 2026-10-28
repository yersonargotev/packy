package capabilitypack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeSkillAssetClosureAtSourceValidation(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Pack)
		want string
	}{
		{"explicit composite", func(*Pack) {}, ""},
		{"plain external asset", func(p *Pack) {
			p.Resources[2].Bindings[0].Capabilities = []SurfaceCapability{}
		}, "outside its complete source tree"},
		{"plain contained asset", func(p *Pack) {
			p.Resources[2].Bindings[0].Capabilities = []SurfaceCapability{}
			p.Resources[1].Source = "skills/workflow/references/guide.md"
		}, ""},
		{"incomplete composite", func(p *Pack) {
			p.Resources[2].Bindings[0].Capabilities[0].ClaudeCompositeSkill.References = []ResourceIdentity{}
		}, "must declare external dependency asset guide in claude-composite-skill references"},
		{"template does not supply external assets", func(p *Pack) {
			p.Resources[2].Bindings[0].Capabilities = []SurfaceCapability{{Type: SurfaceCapabilitySkillTemplate, SkillTemplate: &SkillTemplateCapability{Naming: SkillTemplateNamingNative}}}
		}, "outside its complete source tree"},
		{"competing tree renderers", func(p *Pack) {
			p.Resources[2].Bindings[0].Capabilities = append(p.Resources[2].Bindings[0].Capabilities, SurfaceCapability{Type: SurfaceCapabilitySkillTemplate, SkillTemplate: &SkillTemplateCapability{Naming: SkillTemplateNamingNative}})
		}, "cannot combine skill-template and claude-composite-skill"},
		{"composite destination collision", func(p *Pack) {
			p.Resources[1].Source = "skills/workflow/references/guide.md"
		}, "reference destination"},
		{"effective Claude asset source", func(p *Pack) {
			p.Resources[2].Bindings[0].Capabilities = []SurfaceCapability{}
			p.Resources[1].Source = "skills/workflow/references/guide.md"
			source := "references/guide.md"
			p.Resources[1].Variants = []ResourceVariant{{Surface: SurfaceClaude, Source: &source}}
		}, "outside its complete source tree"},
		{"transitive external asset", func(p *Pack) {
			p.Resources[2].Bindings[0].Capabilities = []SurfaceCapability{}
			p.Resources[2].Requires = []string{"agent:reviewer"}
			p.Resources[0].Requires = []string{"asset:guide"}
			p.Resources[0].Bindings[0].Capabilities = []SurfaceCapability{}
		}, "outside its complete source tree"},
	} {
		t.Run(test.name, func(t *testing.T) {
			pack := validClaudeCapabilityPack()
			test.edit(&pack)
			root := t.TempDir()
			for _, path := range []string{"agents/reviewer.md", "skills/workflow/SKILL.md", pack.Resources[1].Source, "references/guide.md"} {
				file := filepath.Join(root, path)
				if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
					t.Fatal(err)
				}
				content := "reviewed bytes\n"
				if filepath.Base(file) == "SKILL.md" {
					content = "---\nname: workflow\ndescription: Reviewed workflow\n---\n\nreviewed bytes\n"
				}
				if err := os.WriteFile(file, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			err := ValidateClaudeSkillAssetClosures(pack, root)
			if test.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
