// Package testsupport provides deliberately synthetic Pack fixtures for tests
// whose subject is generic capability behavior rather than a real Pack's
// public contract.
//
// The package owns a small typed wire model instead of importing
// capabilitypack or managedpack. Tests in package capabilitypack can therefore
// use these fixtures without creating an import cycle; written manifests are
// still exercised through the production loaders and validators.
package testsupport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
)

type Surface string

const (
	SurfaceClaude   Surface = "claude"
	SurfaceCodex    Surface = "codex"
	SurfaceOpenCode Surface = "opencode"
)

type Relationship string

const (
	RelationshipExactCopy Relationship = "exact-copy"
	RelationshipAdapted   Relationship = "adapted"
)

type Manifest struct {
	SchemaVersion        int        `json:"schema_version"`
	ID                   string     `json:"id"`
	Version              string     `json:"version"`
	Description          string     `json:"description"`
	Selectable           bool       `json:"selectable"`
	Surfaces             []Surface  `json:"surfaces"`
	ReadinessObligations []string   `json:"readiness_obligations"`
	ExternalRequirements []string   `json:"external_requirements"`
	Origins              []Origin   `json:"origins"`
	Resources            []Resource `json:"resources"`
}

type Origin struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Revision   string `json:"revision,omitempty"`
}

type ResourceOrigin struct {
	ID           string       `json:"id"`
	Path         string       `json:"path"`
	Relationship Relationship `json:"relationship"`
}

type Resource struct {
	Kind              string             `json:"kind"`
	ID                string             `json:"id"`
	Source            string             `json:"source,omitempty"`
	Description       string             `json:"description"`
	Mode              string             `json:"mode,omitempty"`
	Tools             []string           `json:"tools"`
	Permissions       []string           `json:"permissions"`
	License           string             `json:"license,omitempty"`
	Attribution       string             `json:"attribution,omitempty"`
	Requires          []string           `json:"requires"`
	Conflicts         []string           `json:"conflicts"`
	Notices           []string           `json:"notices,omitempty"`
	Origin            *ResourceOrigin    `json:"origin,omitempty"`
	Bindings          []Binding          `json:"bindings"`
	SurfaceExclusions []SurfaceExclusion `json:"surface_exclusions"`
}

type Binding struct {
	Surface      Surface      `json:"surface"`
	Projection   string       `json:"projection"`
	Name         string       `json:"name"`
	Invocation   string       `json:"invocation"`
	Mode         string       `json:"mode"`
	Sharing      string       `json:"sharing"`
	Capabilities []Capability `json:"capabilities"`
}

type Capability struct {
	ClaudeAgentDocument           *ClaudeAgentDocument           `json:"claude_agent_document,omitempty"`
	ClaudeCompositeSkill          *ClaudeCompositeSkill          `json:"claude_composite_skill,omitempty"`
	Type                          string                         `json:"type"`
	ExternalExecutableAcquisition *ExternalExecutableAcquisition `json:"external_executable_acquisition,omitempty"`
	PrimaryPrompt                 *SourceCapability              `json:"primary_prompt,omitempty"`
	ProjectInstruction            *SourceCapability              `json:"project_instruction,omitempty"`
}

type ResourceIdentity struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type ClaudeCompositeSkill struct {
	Dependencies []ResourceIdentity `json:"dependencies"`
	References   []ResourceIdentity `json:"references"`
}

type ClaudeAgentDocument struct {
	Skills    []ResourceIdentity `json:"skills"`
	Authority AgentAuthority     `json:"authority"`
}

type AgentAuthority struct {
	PermissionMode string            `json:"permission_mode"`
	Authorities    []AuthorityRecord `json:"authorities"`
}

type AuthorityRecord struct {
	Portable     string   `json:"portable"`
	Declarations []string `json:"declarations"`
	Outcome      string   `json:"outcome"`
	ClaudeTools  []string `json:"claude_tools"`
	Fallback     string   `json:"fallback"`
}

type ExternalExecutableAcquisition struct {
	Tool string `json:"tool"`
}

type SourceCapability struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

type SurfaceExclusion struct {
	Surface Surface `json:"surface"`
	Mode    string  `json:"mode"`
	Code    string  `json:"code"`
	Reason  string  `json:"reason"`
}

// Fixture is one coherent Managed Pack manifest, its declared resource bytes,
// and the external origin bytes required to validate provenance.
type Fixture struct {
	manifest    Manifest
	files       map[string][]byte
	originFiles map[string]map[string][]byte
}

// PortableAllSurfaces returns one instruction that projects to every supported
// surface and requests project-instruction behavior where it is portable.
func PortableAllSurfaces(id string) Fixture {
	source := "instructions/" + id + ".md"
	bytes := []byte("# " + title(id) + " guidance\n\nSynthetic guidance for every supported surface.\n")
	fixture := baseFixture(id, []Surface{SurfaceClaude, SurfaceCodex, SurfaceOpenCode})
	fixture.manifest.Resources = []Resource{
		derivedResource(id, "instruction", "guidance", source, "Portable synthetic guidance", portableBindings("guidance", source)),
		noticeResource(id),
	}
	fixture.files[source] = bytes
	fixture.files[noticePath(id)] = noticeBytes(id)
	fixture.originFiles[originID(id)]["guidance.md"] = append([]byte(nil), bytes...)
	return fixture
}

// CapabilityRich returns a compact fixture with typed primary-prompt and
// project-instruction capabilities. More resource roles are added by focused
// helpers rather than a general scenario builder.
func CapabilityRich(id string) Fixture {
	instructionSource := "instructions/" + id + ".md"
	instructionBytes := []byte("# " + title(id) + " capabilities\n\nSynthetic capability-rich guidance.\n")
	helperSource := "skills/" + id + "-helper"
	helperSkill := []byte("---\nname: helper\ndescription: Synthetic helper skill.\n---\n\nUse the reviewed helper workflow.\n")
	helperReference := []byte("# Helper reference\n\nSynthetic copied-tree reference.\n")
	workflowSource := "skills/" + id + "-workflow"
	workflowSkill := []byte("---\nname: workflow\ndescription: Synthetic composite workflow.\n---\n\nDelegate reviewed work through the helper.\n")
	agentSource := "agents/" + id + "-reviewer.md"
	agentBytes := []byte("---\nname: reviewer\ndescription: Synthetic reviewer agent.\n---\n\nReview the supplied change.\n")
	assetSource := "assets/" + id + "-reference.md"
	assetBytes := []byte("# Workflow reference\n\nSynthetic reference asset.\n")
	fixture := baseFixture(id, []Surface{SurfaceClaude, SurfaceCodex, SurfaceOpenCode})
	fixture.manifest.Resources = []Resource{
		{
			Kind: "agent", ID: "reviewer", Source: agentSource,
			Description: "Synthetic reviewer agent", Mode: "subagent", Tools: []string{}, Permissions: []string{},
			Requires: []string{"skill:helper"}, Conflicts: []string{}, Notices: []string{"notice:apache"},
			Origin: &ResourceOrigin{ID: originID(id), Path: "reviewer.md", Relationship: RelationshipExactCopy},
			Bindings: []Binding{
				binding(SurfaceClaude, "agent", "reviewer", "@reviewer", "exclusive", []Capability{{
					Type: "claude-agent-document",
					ClaudeAgentDocument: &ClaudeAgentDocument{
						Skills:    []ResourceIdentity{{Kind: "skill", ID: "helper"}},
						Authority: AgentAuthority{PermissionMode: "default", Authorities: []AuthorityRecord{}},
					},
				}}),
				binding(SurfaceCodex, "agent", "reviewer", "$reviewer", "exclusive", nil),
				binding(SurfaceOpenCode, "agent", "reviewer", "reviewer", "exclusive", nil),
			},
			SurfaceExclusions: []SurfaceExclusion{},
		},
		derivedResource(id, "asset", "reference", assetSource, "Synthetic workflow reference", []Binding{}),
		derivedResource(id, "instruction", "guidance", instructionSource, "Capability-rich synthetic guidance", []Binding{
			binding(SurfaceClaude, "instruction", "guidance", "guidance", "shared", nil),
			binding(SurfaceCodex, "instruction", "guidance", "guidance", "shared", []Capability{{
				Type: "project-instruction", ProjectInstruction: &SourceCapability{ID: id + "-project", Source: instructionSource},
			}}),
			binding(SurfaceOpenCode, "instruction", "guidance", "guidance", "shared", []Capability{{
				Type: "opencode-primary-prompt", PrimaryPrompt: &SourceCapability{ID: id + "-primary", Source: instructionSource},
			}}),
		}),
		{
			Kind: "lifecycle", ID: "session", Description: "Synthetic runtime session lifecycle",
			Requires: []string{}, Conflicts: []string{},
			Bindings: []Binding{
				binding(SurfaceClaude, "command_hook", "session", "session", "exclusive", nil),
				binding(SurfaceCodex, "lifecycle", "session", "session", "exclusive", nil),
				binding(SurfaceOpenCode, "lifecycle", "session", "session", "exclusive", nil),
			},
			SurfaceExclusions: []SurfaceExclusion{},
		},
		{
			Kind: "notice", ID: "apache", Source: "notices/" + id + "-apache",
			Description: "Synthetic secondary fixture notice", License: "Apache-2.0", Attribution: "Packy Fixture Authors",
			Requires: []string{}, Conflicts: []string{}, Bindings: []Binding{}, SurfaceExclusions: []SurfaceExclusion{},
		},
		noticeResource(id),
		derivedResource(id, "skill", "helper", helperSource, "Synthetic copied-tree helper skill", []Binding{
			binding(SurfaceClaude, "skill", "helper", "/helper", "exclusive", nil),
			binding(SurfaceCodex, "skill", "helper", "$helper", "exclusive", nil),
			binding(SurfaceOpenCode, "skill", "helper", "helper", "exclusive", nil),
		}),
		{
			Kind: "skill", ID: "workflow", Source: workflowSource,
			Description: "Synthetic composite workflow", Requires: []string{"asset:reference", "skill:helper"},
			Conflicts: []string{}, Notices: []string{"notice:apache"},
			Origin: &ResourceOrigin{ID: originID(id), Path: "workflow", Relationship: RelationshipExactCopy},
			Bindings: []Binding{
				binding(SurfaceClaude, "skill", "workflow", "/workflow", "exclusive", []Capability{{
					Type: "claude-composite-skill",
					ClaudeCompositeSkill: &ClaudeCompositeSkill{
						Dependencies: []ResourceIdentity{{Kind: "skill", ID: "helper"}},
						References:   []ResourceIdentity{{Kind: "asset", ID: "reference"}},
					},
				}}),
				binding(SurfaceCodex, "skill", "workflow", "$workflow", "exclusive", nil),
				binding(SurfaceOpenCode, "skill", "workflow", "workflow", "exclusive", nil),
			},
			SurfaceExclusions: []SurfaceExclusion{},
		},
	}
	fixture.files[instructionSource] = instructionBytes
	fixture.files[helperSource+"/SKILL.md"] = helperSkill
	fixture.files[helperSource+"/references/guide.md"] = helperReference
	fixture.files[workflowSource+"/SKILL.md"] = workflowSkill
	fixture.files[agentSource] = agentBytes
	fixture.files[assetSource] = assetBytes
	fixture.files["notices/"+id+"-apache"] = []byte("Apache License 2.0\n\nSynthetic notice for " + id + ".\n")
	fixture.files[noticePath(id)] = noticeBytes(id)
	fixture.originFiles[originID(id)]["guidance.md"] = append([]byte(nil), instructionBytes...)
	fixture.originFiles[originID(id)]["helper/SKILL.md"] = append([]byte(nil), helperSkill...)
	fixture.originFiles[originID(id)]["helper/references/guide.md"] = append([]byte(nil), helperReference...)
	fixture.originFiles[originID(id)]["workflow/SKILL.md"] = append([]byte(nil), workflowSkill...)
	fixture.originFiles[originID(id)]["reviewer.md"] = append([]byte(nil), agentBytes...)
	fixture.originFiles[originID(id)]["reference.md"] = append([]byte(nil), assetBytes...)
	return fixture
}

// ExternalTool returns a Codex skill with one declared external executable and
// its reviewed acquisition capability.
func ExternalTool(id string) Fixture {
	source := "skills/" + id + "-memory"
	bytes := []byte("---\nname: " + id + "-memory\ndescription: Synthetic external-tool fixture.\n---\n\nUse fixture-tool for synthetic memory.\n")
	fixture := baseFixture(id, []Surface{SurfaceCodex})
	fixture.manifest.ExternalRequirements = []string{"engram"}
	fixture.manifest.Resources = []Resource{
		noticeResource(id),
		derivedResource(id, "skill", "memory", source, "Synthetic external-tool skill", []Binding{
			binding(SurfaceCodex, "skill", "memory", "$memory", "exclusive", []Capability{{
				Type: "external-executable-acquisition", ExternalExecutableAcquisition: &ExternalExecutableAcquisition{Tool: "engram"},
			}}),
		}),
	}
	fixture.files[source+"/SKILL.md"] = bytes
	fixture.files[noticePath(id)] = noticeBytes(id)
	fixture.originFiles[originID(id)]["memory/SKILL.md"] = append([]byte(nil), bytes...)
	return fixture
}

// CollisionPair returns two unrelated Packs whose guidance binds to the same
// host-native name while retaining distinct bundle source paths.
func CollisionPair(firstID, secondID string) (Fixture, Fixture) {
	return collisionFixture(firstID), collisionFixture(secondID)
}

func collisionFixture(id string) Fixture {
	fixture := PortableAllSurfaces(id)
	resource := &fixture.manifest.Resources[0]
	resource.ID = id + "-guidance"
	for i := range resource.Bindings {
		resource.Bindings[i].Name = "shared-guidance"
		resource.Bindings[i].Invocation = "shared-guidance"
		for j := range resource.Bindings[i].Capabilities {
			capability := &resource.Bindings[i].Capabilities[j]
			if capability.ProjectInstruction != nil {
				capability.ProjectInstruction.ID = id + "-project"
			}
		}
	}
	return fixture
}

// Manifest returns a deep copy so callers cannot make fixture resources and
// provenance incoherent through an untracked manifest mutation.
func (f Fixture) Manifest() Manifest {
	data, err := json.Marshal(f.manifest)
	if err != nil {
		panic(fmt.Sprintf("marshal synthetic Pack manifest: %v", err))
	}
	var result Manifest
	if err := json.Unmarshal(data, &result); err != nil {
		panic(fmt.Sprintf("clone synthetic Pack manifest: %v", err))
	}
	return result
}

func (f Fixture) CurrentVersion() string {
	return f.manifest.Version
}

// CandidateVersion derives the next patch version from the fixture manifest.
func (f Fixture) CandidateVersion() string {
	current, err := semver.StrictNewVersion(f.manifest.Version)
	if err != nil {
		panic(fmt.Sprintf("invalid synthetic Pack version %q: %v", f.manifest.Version, err))
	}
	return current.IncPatch().String()
}

// Candidate returns an independent fixture at CandidateVersion.
func (f Fixture) Candidate() Fixture {
	return f.WithVersion(f.CandidateVersion())
}

// WithVersion returns an independent fixture with the exact SemVer supplied.
// Invalid versions panic because every Fixture is an in-process test constant;
// allowing an invalid fixture to escape would make later failures misleading.
func (f Fixture) WithVersion(version string) Fixture {
	if _, err := semver.StrictNewVersion(version); err != nil {
		panic(fmt.Sprintf("invalid synthetic Pack version %q: %v", version, err))
	}
	result := f.clone()
	result.manifest.Version = version
	return result
}

// WithExactCopyBytes changes one file in a derived resource and mirrors the
// bytes into its declared origin tree. Use "." when the resource source is a
// file, or a slash-separated path relative to a source directory.
func (f Fixture) WithExactCopyBytes(identity, relative string, data []byte) Fixture {
	result := f.clone()
	resource := result.resource(identity)
	if resource.Origin == nil || resource.Origin.Relationship != RelationshipExactCopy {
		panic(fmt.Sprintf("synthetic Pack resource %q is not an exact-copy", identity))
	}
	resourcePath := rootedPath(resource.Source, relative)
	originPath := rootedPath(resource.Origin.Path, relative)
	if _, ok := result.files[resourcePath]; !ok {
		panic(fmt.Sprintf("synthetic Pack resource %q has no file %q", identity, relative))
	}
	origin := result.originFiles[resource.Origin.ID]
	if _, ok := origin[originPath]; !ok {
		panic(fmt.Sprintf("synthetic Pack resource %q origin has no file %q", identity, relative))
	}
	result.files[resourcePath] = append([]byte(nil), data...)
	origin[originPath] = append([]byte(nil), data...)
	return result
}

// WithAdaptedBytes changes one file in a derived resource, marks the whole
// resource adapted, and requires the existing notice coverage to remain.
func (f Fixture) WithAdaptedBytes(identity, relative string, data []byte) Fixture {
	result := f.clone()
	resource := result.resource(identity)
	if resource.Origin == nil {
		panic(fmt.Sprintf("synthetic Pack resource %q has no origin to adapt", identity))
	}
	if len(resource.Notices) == 0 {
		panic(fmt.Sprintf("synthetic Pack resource %q cannot be adapted without notice coverage", identity))
	}
	resourcePath := rootedPath(resource.Source, relative)
	if _, ok := result.files[resourcePath]; !ok {
		panic(fmt.Sprintf("synthetic Pack resource %q has no file %q", identity, relative))
	}
	result.files[resourcePath] = append([]byte(nil), data...)
	resource.Origin.Relationship = RelationshipAdapted
	return result
}

// WriteBundle materializes a fixture in Packy's current bundle layout. It can
// be called repeatedly on one root to compose unrelated fixtures.
func (f Fixture) WriteBundle(root string) error {
	for relative, data := range f.files {
		if err := writeFile(root, relative, data); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(f.manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal Pack %q manifest: %w", f.manifest.ID, err)
	}
	data = append(data, '\n')
	return writeFile(root, filepath.ToSlash(filepath.Join("packs", f.manifest.ID, "pack.json")), data)
}

// WriteProject materializes a Managed Pack Project and its independent origin
// trees. The returned map implements the data needed by a managedpack origin
// resolver without placing undeclared provenance bytes in the project closure.
func (f Fixture) WriteProject(projectRoot, originsRoot string) (map[string]string, error) {
	for relative, data := range f.files {
		if err := writeFile(projectRoot, relative, data); err != nil {
			return nil, err
		}
	}
	data, err := json.MarshalIndent(f.manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal Pack %q manifest: %w", f.manifest.ID, err)
	}
	if err := writeFile(projectRoot, "pack.json", append(data, '\n')); err != nil {
		return nil, err
	}
	resolved := make(map[string]string, len(f.originFiles))
	for origin, files := range f.originFiles {
		root := filepath.Join(originsRoot, origin)
		if err := os.MkdirAll(root, 0o755); err != nil {
			return nil, fmt.Errorf("create fixture origin %q: %w", origin, err)
		}
		for relative, content := range files {
			if err := writeFile(root, relative, content); err != nil {
				return nil, err
			}
		}
		resolved[origin] = root
	}
	return resolved, nil
}

func (f Fixture) clone() Fixture {
	result := Fixture{
		manifest:    f.Manifest(),
		files:       make(map[string][]byte, len(f.files)),
		originFiles: make(map[string]map[string][]byte, len(f.originFiles)),
	}
	for path, data := range f.files {
		result.files[path] = append([]byte(nil), data...)
	}
	for origin, files := range f.originFiles {
		result.originFiles[origin] = make(map[string][]byte, len(files))
		for path, data := range files {
			result.originFiles[origin][path] = append([]byte(nil), data...)
		}
	}
	return result
}

func (f *Fixture) resource(identity string) *Resource {
	for i := range f.manifest.Resources {
		resource := &f.manifest.Resources[i]
		if resource.Kind+":"+resource.ID == identity {
			return resource
		}
	}
	panic(fmt.Sprintf("synthetic Pack %q has no resource %q", f.manifest.ID, identity))
}

func baseFixture(id string, surfaces []Surface) Fixture {
	origin := originID(id)
	return Fixture{
		manifest: Manifest{
			SchemaVersion: 1, ID: id, Version: "1.0.0",
			Description: "Synthetic " + id + " Pack fixture", Selectable: true,
			Surfaces:             surfaces,
			ReadinessObligations: []string{"runtime-usability", "surface-authorization"},
			ExternalRequirements: []string{},
			Origins: []Origin{{
				ID: origin, Repository: "packy-fixtures/" + id + "-upstream",
				Commit: strings.Repeat("a", 40), Revision: "fixture-v1",
			}},
			Resources: []Resource{},
		},
		files:       map[string][]byte{},
		originFiles: map[string]map[string][]byte{origin: {}},
	}
}

func derivedResource(packID, kind, id, source, description string, bindings []Binding) Resource {
	return Resource{
		Kind: kind, ID: id, Source: source, Description: description,
		Requires: []string{}, Conflicts: []string{}, Notices: []string{"notice:mit"},
		Origin:   &ResourceOrigin{ID: originID(packID), Path: id + pathExtension(source), Relationship: RelationshipExactCopy},
		Bindings: bindings, SurfaceExclusions: []SurfaceExclusion{},
	}
}

func noticeResource(id string) Resource {
	return Resource{
		Kind: "notice", ID: "mit", Source: noticePath(id),
		Description: "Synthetic fixture license notice", License: "MIT", Attribution: "Packy Fixture Authors",
		Requires: []string{}, Conflicts: []string{}, Bindings: []Binding{}, SurfaceExclusions: []SurfaceExclusion{},
	}
}

func portableBindings(id, source string) []Binding {
	return []Binding{
		binding(SurfaceClaude, "instruction", id, id, "shared", nil),
		binding(SurfaceCodex, "instruction", id, id, "shared", []Capability{{
			Type: "project-instruction", ProjectInstruction: &SourceCapability{ID: id, Source: source},
		}}),
		binding(SurfaceOpenCode, "instruction", id, id, "shared", []Capability{{
			Type: "project-instruction", ProjectInstruction: &SourceCapability{ID: id, Source: source},
		}}),
	}
}

func binding(surface Surface, projection, name, invocation, sharing string, capabilities []Capability) Binding {
	if capabilities == nil {
		capabilities = []Capability{}
	}
	return Binding{
		Surface: surface, Projection: projection, Name: name, Invocation: invocation,
		Mode: "native", Sharing: sharing, Capabilities: capabilities,
	}
}

func originID(id string) string   { return id + "-upstream" }
func noticePath(id string) string { return "notices/" + id + "-mit" }

func noticeBytes(id string) []byte {
	return []byte("MIT License\n\nSynthetic notice for " + id + ".\n")
}

func pathExtension(source string) string {
	if strings.HasSuffix(source, ".md") {
		return ".md"
	}
	return ""
}

func rootedPath(root, relative string) string {
	if relative == "." {
		return root
	}
	if relative == "" || filepath.IsAbs(relative) || filepath.ToSlash(filepath.Clean(relative)) != relative || strings.HasPrefix(relative, "../") {
		panic(fmt.Sprintf("synthetic resource file %q must be normalized and relative", relative))
	}
	return filepath.ToSlash(filepath.Join(root, filepath.FromSlash(relative)))
}

func title(id string) string {
	parts := strings.Split(id, "-")
	for i, part := range parts {
		if part != "" {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

func writeFile(root, relative string, data []byte) error {
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create fixture directory for %q: %w", relative, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write fixture file %q: %w", relative, err)
	}
	return nil
}
