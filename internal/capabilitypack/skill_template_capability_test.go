package capabilitypack

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSkillTemplateCapabilitySupportsSkillBindingsOnEverySurface(t *testing.T) {
	for _, surface := range SupportedSurfaces() {
		t.Run(string(surface), func(t *testing.T) {
			binding := Binding{
				Surface: surface, Projection: "skill", Name: "guide", Invocation: nativeSkillInvocation(surface, "guide"),
				Mode: "native", Sharing: "exclusive", Capabilities: []SurfaceCapability{{
					Type:          SurfaceCapabilitySkillTemplate,
					SkillTemplate: &SkillTemplateCapability{Naming: SkillTemplateNamingPackSurface},
				}},
			}
			resource := Resource{Kind: "skill", ID: "guide", Bindings: []Binding{binding}}
			if err := validateBindingV3(resource, binding, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSkillTemplateCapabilityRejectsInvalidOrForeignData(t *testing.T) {
	valid := Binding{
		Surface: SurfaceCodex, Projection: "skill", Name: "guide", Invocation: "$guide",
		Mode: "native", Sharing: "exclusive", Capabilities: []SurfaceCapability{{
			Type:          SurfaceCapabilitySkillTemplate,
			SkillTemplate: &SkillTemplateCapability{Naming: SkillTemplateNamingNative},
		}},
	}
	tests := []struct {
		name string
		edit func(*Resource, *Binding)
		want string
	}{
		{"non-skill", func(r *Resource, _ *Binding) { r.Kind = "command" }, "requires a skill binding"},
		{"non-skill projection", func(_ *Resource, b *Binding) { b.Projection = "instruction" }, "requires a skill binding"},
		{"noncanonical invocation", func(_ *Resource, b *Binding) { b.Invocation = "run guide" }, "canonical native skill invocation"},
		{"missing data", func(_ *Resource, b *Binding) { b.Capabilities[0].SkillTemplate = nil }, "requires skill_template data"},
		{"invalid naming", func(_ *Resource, b *Binding) { b.Capabilities[0].SkillTemplate.Naming = "custom" }, "naming must be native or pack-surface"},
		{"foreign data", func(_ *Resource, b *Binding) {
			b.Capabilities[0].PrimaryPrompt = &PrimaryPromptCapability{ID: "x", Source: "instructions/x.md"}
		}, "does not accept other capability data"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			binding := clonePack(Pack{Resources: []Resource{{Bindings: []Binding{valid}}}}).Resources[0].Bindings[0]
			resource := Resource{Kind: "skill", ID: "guide", Bindings: []Binding{binding}}
			test.edit(&resource, &binding)
			resource.Bindings[0] = binding
			err := validateBindingV3(resource, binding, nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResolvePackForSurfaceAppliesTemplateNamingOnceBeforeAliases(t *testing.T) {
	pack := Pack{ID: "pstack", Resources: []Resource{{
		Kind: "skill", ID: "tdd", Bindings: []Binding{{
			Surface: SurfaceCodex, Projection: "skill", Name: "tdd", Invocation: "$tdd", Capabilities: []SurfaceCapability{{
				Type:          SurfaceCapabilitySkillTemplate,
				SkillTemplate: &SkillTemplateCapability{Naming: SkillTemplateNamingPackSurface},
			}},
		}},
	}}}

	resolved := ResolvePackForSurface(pack, SurfaceCodex)
	resolvedAgain := ResolvePackForSurface(resolved, SurfaceCodex)
	if got, want := resolvedAgain.Resources[0].Bindings[0].Name, "pstack-codex-tdd"; got != want {
		t.Fatalf("resolved name = %q, want %q", got, want)
	}
	if got, want := resolvedAgain.Resources[0].Bindings[0].Invocation, "$pstack-codex-tdd"; got != want {
		t.Fatalf("resolved invocation = %q, want %q", got, want)
	}
	aliased := resourceWithSurfaceAlias(resolvedAgain.Resources[0], []SurfaceAlias{{Kind: "skill", ID: "tdd", Name: "red-green"}}, SurfaceCodex)
	if got := aliased.Bindings[0].Name; got != "red-green" {
		t.Fatalf("aliased name = %q", got)
	}
	if got := aliased.Bindings[0].Invocation; got != "$red-green" {
		t.Fatalf("aliased invocation = %q", got)
	}
	if got := pack.Resources[0].Bindings[0].Name; got != "tdd" {
		t.Fatalf("canonical pack was mutated: %q", got)
	}
}

func TestSkillTemplateNamesIncludesEffectiveOrdinarySkillDependencies(t *testing.T) {
	pack := Pack{ID: "pstack", Resources: []Resource{
		{Kind: "skill", ID: "helper", Bindings: []Binding{{Surface: SurfaceClaude, Projection: "skill", Name: "helper"}}},
		{Kind: "skill", ID: "tdd", Bindings: []Binding{{Surface: SurfaceClaude, Projection: "skill", Name: "tdd", Capabilities: []SurfaceCapability{{Type: SurfaceCapabilitySkillTemplate, SkillTemplate: &SkillTemplateCapability{Naming: SkillTemplateNamingPackSurface}}}}}}}}
	want := map[string]string{"helper": "helper", "tdd": "pstack-claude-tdd"}
	if got := SkillTemplateNames(ResolvePackForSurface(pack, SurfaceClaude), SurfaceClaude); !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %#v, want %#v", got, want)
	}
}

func TestSkillTemplateNamingPreservesHostInvocationSyntax(t *testing.T) {
	for _, surface := range SupportedSurfaces() {
		t.Run(string(surface), func(t *testing.T) {
			pack := Pack{ID: "pstack", Resources: []Resource{{
				Kind: "skill", ID: "tdd", Bindings: []Binding{{
					Surface: surface, Projection: "skill", Name: "tdd", Invocation: nativeSkillInvocation(surface, "tdd"),
					Capabilities: []SurfaceCapability{{Type: SurfaceCapabilitySkillTemplate, SkillTemplate: &SkillTemplateCapability{Naming: SkillTemplateNamingPackSurface}}},
				}},
			}}}
			resolved := ResolvePackForSurface(pack, surface)
			binding := resolved.Resources[0].Bindings[0]
			if got, want := binding.Invocation, nativeSkillInvocation(surface, "pstack-"+string(surface)+"-tdd"); got != want {
				t.Fatalf("resolved invocation = %q, want %q", got, want)
			}
		})
	}
}

func TestSkillTemplateLifecycleMetadataUsesEffectiveNamesAndAliases(t *testing.T) {
	for _, surface := range SupportedSurfaces() {
		t.Run(string(surface), func(t *testing.T) {
			pack := Pack{ID: "pstack", Surfaces: []Surface{surface}, Resources: []Resource{{
				Kind: "skill", ID: "tdd", Bindings: []Binding{{
					Surface: surface, Projection: "skill", Name: "tdd", Invocation: nativeSkillInvocation(surface, "tdd"),
					Mode: "native", Sharing: "exclusive", Capabilities: []SurfaceCapability{{Type: SurfaceCapabilitySkillTemplate, SkillTemplate: &SkillTemplateCapability{Naming: SkillTemplateNamingPackSurface}}},
				}},
			}}}
			contract := LifecycleContractFor(pack, surface, nil)
			wantName := "pstack-" + string(surface) + "-tdd"
			if len(contract.Bindings) != 1 || contract.Bindings[0].Name != wantName || contract.Bindings[0].Invocation != nativeSkillInvocation(surface, wantName) {
				t.Fatalf("resolved lifecycle binding = %#v", contract.Bindings)
			}
			aliased := LifecycleContractFor(pack, surface, []SurfaceAlias{{Kind: "skill", ID: "tdd", Name: "red-green"}})
			if len(aliased.Bindings) != 1 || aliased.Bindings[0].Name != "red-green" || aliased.Bindings[0].Invocation != nativeSkillInvocation(surface, "red-green") {
				t.Fatalf("aliased lifecycle binding = %#v", aliased.Bindings)
			}
		})
	}
}

func TestSkillTemplateGeneratedNameHasPortableLimit(t *testing.T) {
	pack := Pack{ID: strings.Repeat("a", 50), Resources: []Resource{{
		Kind: "skill", ID: "guide", Bindings: []Binding{{
			Surface: SurfaceOpenCode, Projection: "skill", Name: "guide", Capabilities: []SurfaceCapability{{
				Type:          SurfaceCapabilitySkillTemplate,
				SkillTemplate: &SkillTemplateCapability{Naming: SkillTemplateNamingPackSurface},
			}},
		}},
	}}}
	err := validateSkillTemplateNames(pack)
	if err == nil || !strings.Contains(err.Error(), "exceeds 64 characters") {
		t.Fatalf("limit error = %v", err)
	}
}

func TestClonePackCopiesSkillTemplateCapability(t *testing.T) {
	pack := Pack{Resources: []Resource{{Bindings: []Binding{{Capabilities: []SurfaceCapability{{
		Type: SurfaceCapabilitySkillTemplate, SkillTemplate: &SkillTemplateCapability{Naming: SkillTemplateNamingNative},
	}}}}}}}
	cloned := clonePack(pack)
	cloned.Resources[0].Bindings[0].Capabilities[0].SkillTemplate.Naming = SkillTemplateNamingPackSurface
	if got := pack.Resources[0].Bindings[0].Capabilities[0].SkillTemplate.Naming; got != SkillTemplateNamingNative {
		t.Fatalf("clone mutated source capability: %q", got)
	}
}

func TestValidateSkillTemplateSourcesAcceptsReviewedDirectReferencesWithoutMutatingSource(t *testing.T) {
	root := t.TempDir()
	tddBody := "---\nname: tdd\n---\nUse ${{packy.skill:teach}} or /{{packy.skill:tdd}}.\n"
	writeSkillTemplateFixture(t, root, "skills/tdd", tddBody)
	writeSkillTemplateFixture(t, root, "skills/teach", "---\nname: teach\n---\nTeach.\n")
	pack := skillTemplateValidationPack(root, []string{"skill:teach"})

	if err := validateSkillTemplateSources(pack, root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "skills/tdd/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != tddBody {
		t.Fatalf("validation mutated source: %q", data)
	}
}

func TestValidateSkillTemplateSourcesRejectsUnsafeReferenceContracts(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		requires  []string
		configure func(*Pack)
		want      string
	}{
		{
			name: "directive without capability", body: "---\nname: tdd\n---\n{{packy.skill:teach}}\n", requires: []string{"skill:teach"},
			configure: func(pack *Pack) { pack.Resources[0].Bindings[0].Capabilities = []SurfaceCapability{} },
			want:      `without the "skill-template" capability`,
		},
		{
			name: "malformed directive", body: "---\nname: tdd\n---\n{{packy.skill:Teach}}\n", requires: []string{"skill:teach"},
			want: "invalid logical ID",
		},
		{
			name: "invalid frontmatter", body: "---\ndescription: missing name\n---\nBody.\n", requires: []string{"skill:teach"},
			want: "missing a name scalar",
		},
		{
			name: "missing direct dependency", body: "---\nname: tdd\n---\n{{packy.skill:teach}}\n", requires: []string{},
			want: "without a direct requires dependency",
		},
		{
			name: "target excluded", body: "---\nname: tdd\n---\n{{packy.skill:teach}}\n", requires: []string{"skill:teach"},
			configure: func(pack *Pack) {
				pack.Resources[1].Bindings = nil
				pack.Resources[1].SurfaceExclusions = []SurfaceExclusion{{Surface: SurfaceCodex, Mode: "mandatory", Code: "unsupported", Reason: "Unavailable"}}
			},
			want: "absent, excluded, or unbound skill",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeSkillTemplateFixture(t, root, "skills/tdd", test.body)
			writeSkillTemplateFixture(t, root, "skills/teach", "---\nname: teach\n---\nTeach.\n")
			pack := skillTemplateValidationPack(root, test.requires)
			if test.configure != nil {
				test.configure(&pack)
			}
			err := validateSkillTemplateSources(pack, root)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateSkillTemplateSourcesUsesEffectiveVariantOnly(t *testing.T) {
	root := t.TempDir()
	writeSkillTemplateFixture(t, root, "skills/tdd", "---\nname: tdd\n---\n{{packy.skill:Not-Valid}}\n")
	writeSkillTemplateFixture(t, root, "skills/tdd-codex", "---\nname: tdd\n---\nCodex body.\n")
	writeSkillTemplateFixture(t, root, "skills/teach", "---\nname: teach\n---\nTeach.\n")
	pack := skillTemplateValidationPack(root, []string{"skill:teach"})
	variantSource := "skills/tdd-codex"
	pack.Resources[0].Variants = ResourceVariants{{Surface: SurfaceCodex, Source: &variantSource}}

	if err := validateSkillTemplateSources(pack, root); err != nil {
		t.Fatalf("effective variant validation: %v", err)
	}
}

func skillTemplateValidationPack(root string, requires []string) Pack {
	return Pack{ID: "pstack", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
		{
			Kind: "skill", ID: "tdd", Source: "skills/tdd", Requires: requires, catalogRoot: root,
			Bindings: []Binding{{
				Surface: SurfaceCodex, Projection: "skill", Name: "tdd", Capabilities: []SurfaceCapability{{
					Type: SurfaceCapabilitySkillTemplate, SkillTemplate: &SkillTemplateCapability{Naming: SkillTemplateNamingPackSurface},
				}},
			}},
		},
		{
			Kind: "skill", ID: "teach", Source: "skills/teach", catalogRoot: root,
			Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "teach", Capabilities: []SurfaceCapability{}}},
		},
	}}
}

func writeSkillTemplateFixture(t *testing.T, root, source, body string) {
	t.Helper()
	directory := filepath.Join(root, filepath.FromSlash(source))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestHydrateProjectLockPreservesReceiptedEffectiveSkillName(t *testing.T) {
	lock, err := hydrateProjectLock(ProjectLockProposal{
		SchemaVersion: 1,
		Receipts: []installedPackReceipt{{
			Pack: installedPackIdentity{ID: "pstack", Version: "1.0.0"}, Surface: SurfaceCodex,
			ReadinessObligations: []ReadinessObligation{}, ExternalRequirements: []string{},
			Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, Aliases: []SurfaceAlias{},
			Resources:   []ResourceIdentity{{Kind: "skill", ID: "tdd"}},
			Projections: []installedProjection{{ID: "skill:tdd", Target: ".agents/skills/pstack-codex-tdd", Digest: strings.Repeat("a", 64), FileMode: 0o755}},
			Sensitive:   []ProjectSensitiveDisclosure{},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Bindings) != 1 || lock.Bindings[0].Name != "pstack-codex-tdd" || lock.Bindings[0].Invocation != "$pstack-codex-tdd" {
		t.Fatalf("hydrated bindings = %#v", lock.Bindings)
	}
}
