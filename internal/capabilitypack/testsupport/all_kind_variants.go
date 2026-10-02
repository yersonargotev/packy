package testsupport

import (
	"sort"
	"strings"
)

// AllKindVariants exercises all eight logical kinds with reviewed host bodies.
// Bindings remain fixed, including each host's explicit capability sources.
func AllKindVariants(id string) Fixture {
	f := baseFixture(id, []Surface{SurfaceClaude, SurfaceCodex, SurfaceOpenCode})
	f.manifest.Origins = []Origin{}
	kinds := []string{"agent", "asset", "command", "instruction", "lifecycle", "mcp_server", "notice", "skill"}
	roots := map[string]string{"agent": "agents", "asset": "assets", "command": "commands", "instruction": "instructions", "notice": "notices", "skill": "skills"}
	for _, kind := range kinds {
		r := Resource{Kind: kind, ID: "sample-" + strings.ReplaceAll(kind, "_", "-"), Description: "Common " + kind, Requires: []string{}, Conflicts: []string{}, Bindings: []Binding{}, SurfaceExclusions: []SurfaceExclusion{}}
		if kind == "agent" {
			r.Mode = "subagent"
			r.Tools = []string{}
			r.Permissions = []string{}
		}
		if kind == "command" {
			r.Arguments = &CommandArguments{Mode: "none"}
			r.Requires = []string{"asset:sample-asset"}
		}
		if kind == "notice" {
			r.License = "MIT"
			r.Attribution = "Original Authors"
		} else {
			r.Notices = []string{"notice:sample-notice"}
		}
		if kind == "mcp_server" {
			r.Command = "common-server"
			r.Args = []string{"common-argument"}
		}
		for _, body := range []string{"common", "claude", "codex", "opencode"} {
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
				if kind == "command" && body == "claude" {
					data = "description = \"Reviewed Claude command\"\nprompt = \"Reviewed claude command\"\n"
				}
				f.files[file] = []byte(data)
			}
			if body == "common" {
				r.Source = source
				continue
			}
			surface := Surface(body)
			v := ResourceVariant{Surface: surface, Source: source, Description: "Reviewed " + body + " " + kind}
			if kind == "mcp_server" {
				v.Command = body + "-server"
				args := []string{body + "-argument"}
				v.Args = &args
			}
			if kind == "agent" && body != "claude" {
				tools := []string{"read"}
				permissions := []string{"filesystem"}
				v.Tools = &tools
				v.Permissions = &permissions
				v.Mode = "primary"
			}
			if kind == "command" {
				v.Arguments = &CommandArguments{Mode: "freeform", Placeholder: "$ARGUMENTS"}
			}
			if kind == "lifecycle" {
				requires := []string{"skill:sample-skill"}
				v.Requires = &requires
			}
			if kind == "notice" {
				v.Attribution = "Original Authors; " + body + " adaptation"
			}
			r.Variants = append(r.Variants, v)
			if kind == "asset" || kind == "notice" {
				continue
			}
			projection, invocation := kind, r.ID
			if kind == "command" && body != "opencode" {
				projection = "skill"
			}
			if kind == "command" {
				invocation = "/" + r.ID
			}
			if kind == "lifecycle" && body == "claude" {
				projection = "command_hook"
			}
			b := binding(surface, projection, r.ID, invocation, "exclusive", nil)
			if kind == "command" && body == "codex" {
				b.Mode = "degraded"
				b.Degradation = "codex-command-as-workflow-skill"
				b.Invocation = "$" + r.ID
			}
			if kind == "instruction" {
				b.Sharing = "shared"
				if body != "claude" {
					b.Capabilities = []Capability{{Type: "project-instruction", ProjectInstruction: &SourceCapability{ID: r.ID, Source: source}}}
				}
			}
			if kind == "command" && body == "claude" {
				b.Capabilities = []Capability{{Type: "claude-composite-skill", ClaudeCompositeSkill: &ClaudeCompositeSkill{Dependencies: []ResourceIdentity{}, References: []ResourceIdentity{{Kind: "asset", ID: "sample-asset"}}}}}
			}
			if kind == "lifecycle" && body == "claude" {
				b.Hook = &CommandHook{Type: "command", Event: "SessionStart", Command: "reviewed-hook", Args: []string{"fixed-binding"}, TimeoutSeconds: 5, Failure: "warn", Authorities: []string{}}
			}
			r.Bindings = append(r.Bindings, b)
		}
		f.manifest.Resources = append(f.manifest.Resources, r)
	}
	sort.Slice(f.manifest.Resources, func(i, j int) bool { return f.manifest.Resources[i].Kind < f.manifest.Resources[j].Kind })
	f.operational = ResourceIdentity{Kind: "skill", ID: "sample-skill"}
	return f
}

// WithAllKindVariantRevision changes reviewed bodies and typed MCP arguments.
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
				if r.Kind == "command" && surface == SurfaceClaude {
					result.files[file] = []byte("description = \"Reviewed Claude command\"\nprompt = \"Reviewed claude command Updated reviewed body\"\n")
				} else {
					result.files[file] = append(result.files[file], []byte("Updated reviewed body\n")...)
				}
			}
			if r.Kind == "mcp_server" {
				args := []string{string(surface) + "-argument", "updated"}
				v.Args = &args
			}
		}
	}
	return result
}
