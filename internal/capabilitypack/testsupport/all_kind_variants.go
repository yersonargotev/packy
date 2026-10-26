package testsupport

import (
	"sort"
	"strings"
)

// AllKindVariants exercises all supported logical kinds with reviewed host bodies.
// Bindings remain fixed, including each host's explicit capability sources.
func AllKindVariants(id string) Fixture {
	f := baseFixture(id, []Surface{SurfaceClaude, SurfaceAgents})
	f.manifest.Origins = []Origin{}
	kinds := []string{"agent", "asset", "instruction", "notice", "skill"}
	roots := map[string]string{"agent": "agents", "asset": "assets", "instruction": "instructions", "notice": "notices", "skill": "skills"}
	for _, kind := range kinds {
		r := Resource{Kind: kind, ID: "sample-" + strings.ReplaceAll(kind, "_", "-"), Description: "Common " + kind, Requires: []string{}, Conflicts: []string{}, Bindings: []Binding{}, SurfaceExclusions: []SurfaceExclusion{}}
		if kind == "agent" {
			r.Mode = "subagent"
			r.Requires = []string{"asset:sample-asset"}
			r.Tools = []string{}
			r.Permissions = []string{}
		}
		if kind == "notice" {
			r.License = "MIT"
			r.Attribution = "Original Authors"
		} else {
			r.Notices = []string{"notice:sample-notice"}
		}
		for _, body := range []string{"common", "claude", "agents"} {
			source := ""
			if root := roots[kind]; root != "" {
				source = root + "/" + body
				file := source
				if kind == "skill" {
					file += "/SKILL.md"
				} else {
					source += ".md"
					file = source
				}
				data := "Reviewed " + body + " " + kind + "\n"
				if kind == "notice" {
					data = "Original MIT notice and attribution.\n"
					if body != "common" {
						data += "Reviewed " + body + " notice\n"
					}
				}
				if kind == "skill" {
					data = "---\nname: sample-skill\ndescription: Reviewed skill.\n---\n\n" + data
				}
				f.files[file] = []byte(data)
			}
			if body == "common" {
				r.Source = source
				continue
			}
			surface := Surface(body)
			v := ResourceVariant{Surface: surface, Source: source, Description: "Reviewed " + body + " " + kind}
			if kind == "agent" && body != "claude" {
				tools := []string{"read"}
				permissions := []string{"filesystem"}
				v.Tools = &tools
				v.Permissions = &permissions
				v.Mode = "primary"
			}
			if kind == "notice" {
				v.Attribution = "Original Authors; " + body + " adaptation"
			}
			r.Variants = append(r.Variants, v)
			if kind == "asset" || kind == "notice" {
				continue
			}
			projection, invocation := kind, r.ID
			b := binding(surface, projection, r.ID, invocation, "exclusive", nil)
			if kind == "agent" && body == "agents" {
				b.ClientProjection = &ClientProjection{Client: "codex", Scopes: []string{"global"}}
			}
			if kind == "instruction" {
				b.Sharing = "shared"
				if body != "claude" {
					b.Capabilities = []Capability{{Type: "project-instruction", ProjectInstruction: &SourceCapability{ID: r.ID, Source: source}}}
				}
			}
			r.Bindings = append(r.Bindings, b)
		}
		f.manifest.Resources = append(f.manifest.Resources, r)
	}
	sort.Slice(f.manifest.Resources, func(i, j int) bool { return f.manifest.Resources[i].Kind < f.manifest.Resources[j].Kind })
	f.operational = ResourceIdentity{Kind: "skill", ID: "sample-skill"}
	return f
}

// WithAllKindVariantRevision changes reviewed bodies.
func (f Fixture) WithAllKindVariantRevision(surface Surface) Fixture {
	result := f.clone()
	result.manifest.Version = "2.0.0"
	for i := range result.manifest.Resources {
		r := &result.manifest.Resources[i]
		for j := range r.Variants {
			v := &r.Variants[j]
			if v.Surface != surface {
				continue
			}
			if v.Source != "" {
				file := v.Source
				if r.Kind == "skill" {
					file += "/SKILL.md"
				}
				result.files[file] = append(result.files[file], []byte("Updated reviewed body\n")...)
			}
		}
	}
	return result
}
