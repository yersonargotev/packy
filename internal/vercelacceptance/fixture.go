// Package vercelacceptance owns the detached, inert Vercel contract oracle.
// It is acceptance data only: it is neither catalog registration nor executable content.
package vercelacceptance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/packsync"
)

const PrimaryCommit = "7c180d9044c9ae2b442b567aad4e42a28dd5ed62"

type BlobIdentity struct {
	SourceID, Path, Commit, GitBlob string
	Size                            int
	SHA256                          string
}
type LoaderAdaptation struct {
	ResourceID, AssetID, PackageRelativePath string
	OriginalSHA256, AdaptedSHA256            string
}
type LegalEvidence struct{ SourceID, Disposition, EvidenceID, EvidenceSHA256 string }
type Compatibility struct {
	Introduction, ProvenanceOnly               string
	PatchPreserves, MinorAllows, MajorIncludes []string
}
type AliasPolicy struct {
	InitialAliases           []string
	Selection                string
	SuggestedPattern         string
	UnmanagedCollision       string
	PreservesLogicalIdentity bool
}
type Fixture struct {
	Pack              capabilitypack.Pack `json:"-"`
	Sources           packsync.Config     `json:"sources"`
	Blobs             []BlobIdentity      `json:"blobs"`
	Loaders           []LoaderAdaptation  `json:"loaders"`
	Legal             []LegalEvidence     `json:"legal"`
	Compatibility     Compatibility       `json:"compatibility"`
	Aliases           AliasPolicy         `json:"aliases"`
	SnapshotSHA256    string              `json:"snapshot_sha256"`
	CatalogSelectable bool                `json:"catalog_selectable"`
}

var skills = []struct{ id, upstream, public string }{
	{"vercel-composition-patterns", "composition-patterns", "vercel-composition-patterns"}, {"vercel-deploy-to-vercel", "deploy-to-vercel", "deploy-to-vercel"},
	{"vercel-react-best-practices", "react-best-practices", "vercel-react-best-practices"}, {"vercel-react-native-skills", "react-native-skills", "vercel-react-native-skills"},
	{"vercel-react-view-transitions", "react-view-transitions", "vercel-react-view-transitions"}, {"vercel-cli-with-tokens", "vercel-cli-with-tokens", "vercel-cli-with-tokens"},
	{"vercel-optimize", "vercel-optimize", "vercel-optimize"}, {"vercel-web-design-guidelines", "web-design-guidelines", "web-design-guidelines"},
	{"vercel-writing-guidelines", "writing-guidelines", "writing-guidelines"},
}

func Canonical() Fixture {
	resources := make([]capabilitypack.Resource, 0, 13)
	for _, s := range skills {
		req := []string{}
		if s.id == "vercel-web-design-guidelines" {
			req = []string{"asset:web-interface-guidelines-rules"}
		}
		if s.id == "vercel-writing-guidelines" {
			req = []string{"asset:writing-guidelines-rules"}
		}
		resources = append(resources, capabilitypack.Resource{Kind: "skill", ID: s.id, Source: "skills/" + s.id, Requires: req, Bindings: bindings(s.public), SurfaceExclusions: []capabilitypack.SurfaceExclusion{}, RuntimeModes: modes(s.id)})
	}
	resources = append(resources,
		capabilitypack.Resource{Kind: "asset", ID: "web-interface-guidelines-rules", Source: "references/vercel-web-interface-guidelines-command.md", Requires: []string{}, Bindings: []capabilitypack.Binding{}, SurfaceExclusions: []capabilitypack.SurfaceExclusion{}},
		capabilitypack.Resource{Kind: "asset", ID: "writing-guidelines-rules", Source: "references/vercel-writing-guidelines-command.md", Requires: []string{}, Bindings: []capabilitypack.Binding{}, SurfaceExclusions: []capabilitypack.SurfaceExclusion{}},
		capabilitypack.Resource{Kind: "notice", ID: "web-interface-guidelines-mit", Source: "notices/vercel-web-interface-guidelines-MIT.txt", Requires: []string{}, Bindings: []capabilitypack.Binding{}, SurfaceExclusions: []capabilitypack.SurfaceExclusion{}, License: "MIT", Attribution: "Copyright (c) 2025 Vercel Labs"},
		capabilitypack.Resource{Kind: "notice", ID: "writing-guidelines-mit", Source: "notices/vercel-writing-guidelines-MIT.txt", Requires: []string{}, Bindings: []capabilitypack.Binding{}, SurfaceExclusions: []capabilitypack.SurfaceExclusion{}, License: "MIT", Attribution: "Copyright (c) 2026 Vercel Labs"})
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Kind != resources[j].Kind {
			return resources[i].Kind < resources[j].Kind
		}
		return resources[i].ID < resources[j].ID
	})
	exclusions := []capabilitypack.Exclusion{{ID: "excluded-upstream-archives", SourcePaths: []string{"react-best-practices.zip", "react-native-skills.zip", "skills/deploy-to-vercel/Archive.zip", "vercel-composition-patterns.zip", "vercel-deploy-claimable.zip", "vercel-react-best-practices.zip"}, Reason: "duplicate archive content is outside the selected complete trees"}}
	pack := capabilitypack.Pack{ID: "vercel", Version: "1.0.0", Surfaces: []capabilitypack.Surface{"claude", "codex", "opencode"}, Provides: []string{"workflow:vercel"}, Requires: capabilitypack.Requirements{Capabilities: []string{}, Tools: []string{}}, Conflicts: []string{}, Resources: resources, Contract: capabilitypack.Contract{Exclusions: exclusions}}
	src := sourceConfigs()
	loaders := []LoaderAdaptation{
		{"vercel-web-design-guidelines", "web-interface-guidelines-rules", "../../references/vercel-web-interface-guidelines-command.md", "f4647ca866a3accf763777f83e7682954f0187cd6bea7eea0399796652414e8f", "d7d939ec1312895cb4e42b420233a7bf3e7a5f72c3b98b3b5f9a21c56e90ac2c"},
		{"vercel-writing-guidelines", "writing-guidelines-rules", "../../references/vercel-writing-guidelines-command.md", "89a5f581193289b80af58b980090aeed535047c8df2b55ccbaae0de40283a99d", "91d3b9d111d0e685e6081798b660d33179b2f842cce2423b94107bf0be58c034"},
	}
	return Fixture{Pack: pack, Sources: packsync.Config{SchemaVersion: 1, Sources: src}, Blobs: []BlobIdentity{
		{"vercel-agent-skills", "README.md", PrimaryCommit, "daecfea1e60f8f045a3d711c605d70edcdf9d92a", 7538, "c0a05286fc2a9d52ec2480bf070867665b9357beef37c9c2812e5b2ece571b6a"},
		{"vercel-web-interface-guidelines", "command.md", "4e799d45c17aec1498c269287a83b9dba22b966b", "c6d1a9064f8a8615e8a9a8c50590f80a34545d1d", 6939, "eea73cb6dd46fee9faec9973e8e7fe198b5f07ec326f14d276a56e50287e1cab"},
		{"vercel-web-interface-guidelines", "LICENSE", "4e799d45c17aec1498c269287a83b9dba22b966b", "b3575a3c1358eac4b9ee36a4c851872d81417760", 1068, "6cd1609c9c12233507cdd2ce0d32e9a721e3c27494951be06b90090deeeb7af2"},
		{"vercel-writing-guidelines", "command.md", "83e2316b034cf572400513538e4e4da01c4cc742", "8452139a442bef9c25abdd19ed9d4b0ef93aab02", 14228, "fb638d7821bb4472e4492aedcfb51f2636c7d31d34ff9f01cca5bcdce9b1841f"},
		{"vercel-writing-guidelines", "LICENSE", "83e2316b034cf572400513538e4e4da01c4cc742", "094e15e1beb5b639309cc5a920e9b85d2be725ce", 1068, "7ecf613390251c6a08d66982519db39f2ae7fc2e474c65630adea78e84dc4445"}}, Loaders: loaders,
		Legal: []LegalEvidence{{"vercel-agent-skills", "redistributable", "vercel-agent-skills-7c180d9-readme-mit", "e98ea93b2fc7ee5e4b49364ab0fc4e13fe4b0801d6439bd7e07180a7751e6dc3"}, {"vercel-web-interface-guidelines", "MIT", "LICENSE", "6cd1609c9c12233507cdd2ce0d32e9a721e3c27494951be06b90090deeeb7af2"}, {"vercel-writing-guidelines", "MIT", "LICENSE", "7ecf613390251c6a08d66982519db39f2ae7fc2e474c65630adea78e84dc4445"}},
		Compatibility: Compatibility{
			Introduction:   "absent->1.0.0 initial registration, not a migration",
			ProvenanceOnly: "no-op when selected bytes and legal obligations are identical",
			PatchPreserves: []string{"resources", "names", "invocations", "projections", "requirements", "authorities", "effects", "availability semantics", "fallbacks", "exclusions", "legal obligations", "mandatory actions"},
			MinorAllows:    []string{"compatible independent modes", "relaxed requirements", "verified fallbacks", "reduced authority or effects while preserving the logical result", "no migration", "no mandatory action"},
			MajorIncludes:  []string{"resource removal or rename", "strengthened requirements or versions", "broadened authority or effects", "removed fallback", "weakened redaction or fail-before-effects safety", "incompatible legal change", "migration", "mandatory action"},
		},
		Aliases: AliasPolicy{
			InitialAliases:           []string{},
			Selection:                "explicit and surface-local",
			SuggestedPattern:         "vercel-pack-<public-name>",
			UnmanagedCollision:       "block without adoption, overwrite, precedence, or silent rename",
			PreservesLogicalIdentity: true,
		},
		SnapshotSHA256: ExactArchiveSHA256, CatalogSelectable: false}
}

func bindings(name string) []capabilitypack.Binding {
	return []capabilitypack.Binding{{Surface: "claude", Projection: "skill", Name: name, Invocation: "/" + name, Mode: "native", Sharing: "exclusive"}, {Surface: "codex", Projection: "skill", Name: name, Invocation: "$" + name, Mode: "native", Sharing: "exclusive"}, {Surface: "opencode", Projection: "skill", Name: name, Invocation: name, Mode: "native", Sharing: "exclusive"}}
}
func sourceConfigs() []packsync.SourceConfig {
	primary := packsync.SourceConfig{ID: "vercel-agent-skills", Provider: "github", Repository: "vercel-labs/agent-skills", Selector: packsync.Selector{Mode: packsync.SelectorCommit, Ref: PrimaryCommit}}
	for _, s := range skills {
		primary.Resources = append(primary.Resources, packsync.Binding{PackID: "vercel", Kind: "skill", ResourceID: s.id, UpstreamPath: "skills/" + s.upstream})
	}
	return []packsync.SourceConfig{primary, {ID: "vercel-web-interface-guidelines", Provider: "github", Repository: "vercel-labs/web-interface-guidelines", Selector: packsync.Selector{Mode: packsync.SelectorCommit, Ref: "4e799d45c17aec1498c269287a83b9dba22b966b"}, Resources: []packsync.Binding{{PackID: "vercel", Kind: "asset", ResourceID: "web-interface-guidelines-rules", UpstreamPath: "command.md"}, {PackID: "vercel", Kind: "notice", ResourceID: "web-interface-guidelines-mit", UpstreamPath: "LICENSE"}}}, {ID: "vercel-writing-guidelines", Provider: "github", Repository: "vercel-labs/writing-guidelines", Selector: packsync.Selector{Mode: packsync.SelectorCommit, Ref: "83e2316b034cf572400513538e4e4da01c4cc742"}, Resources: []packsync.Binding{{PackID: "vercel", Kind: "asset", ResourceID: "writing-guidelines-rules", UpstreamPath: "command.md"}, {PackID: "vercel", Kind: "notice", ResourceID: "writing-guidelines-mit", UpstreamPath: "LICENSE"}}}}
}

func CanonicalManifestJSON() ([]byte, error) {
	return capabilitypack.EncodePortableManifestV4(Canonical().Pack)
}
func CanonicalJSON() ([]byte, error) {
	f := Canonical()
	manifest, err := CanonicalManifestJSON()
	if err != nil {
		return nil, err
	}
	var m any
	if err = json.Unmarshal(manifest, &m); err != nil {
		return nil, err
	}
	wire := struct {
		Manifest          any                `json:"manifest"`
		Sources           packsync.Config    `json:"sources"`
		Blobs             []BlobIdentity     `json:"blobs"`
		Loaders           []LoaderAdaptation `json:"loaders"`
		Legal             []LegalEvidence    `json:"legal"`
		Compatibility     Compatibility      `json:"compatibility"`
		Aliases           AliasPolicy        `json:"aliases"`
		SnapshotSHA256    string             `json:"snapshot_sha256"`
		CatalogSelectable bool               `json:"catalog_selectable"`
	}{m, f.Sources, f.Blobs, f.Loaders, f.Legal, f.Compatibility, f.Aliases, f.SnapshotSHA256, f.CatalogSelectable}
	b, err := json.MarshalIndent(wire, "", "  ")
	return append(b, '\n'), err
}
func Digest() (string, error) {
	b, e := CanonicalJSON()
	if e != nil {
		return "", e
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

func NegativeTwin(fact string) (Fixture, error) {
	f := Canonical()
	switch fact {
	case "missing":
		f.Pack.Resources = f.Pack.Resources[1:]
	case "duplicate":
		f.Pack.Resources = append(f.Pack.Resources, f.Pack.Resources[0])
	case "stale":
		f.Sources.Sources[0].Selector.Ref = strings.Repeat("0", 40)
	case "unauthorized":
		f.Legal[0].Disposition = "blocked"
	case "moving":
		f.Sources.Sources[1].Selector.Ref = "main"
	case "undeclared":
		for i := range f.Pack.Resources {
			if f.Pack.Resources[i].ID == "vercel-web-design-guidelines" {
				f.Pack.Resources[i].Requires = []string{}
			}
		}
	case "missing-notice":
		for i, resource := range f.Pack.Resources {
			if resource.Kind == "notice" {
				f.Pack.Resources = append(f.Pack.Resources[:i], f.Pack.Resources[i+1:]...)
				break
			}
		}
	case "missing-binding":
		for i := range f.Pack.Resources {
			if len(f.Pack.Resources[i].Bindings) > 0 {
				f.Pack.Resources[i].Bindings = f.Pack.Resources[i].Bindings[1:]
				break
			}
		}
	default:
		return Fixture{}, fmt.Errorf("unknown negative fact %q", fact)
	}
	return f, nil
}

// Validate applies the generic portable-manifest and Pack Source validators,
// then requires the complete fixed Vercel candidate. It is pure and returns
// stable acceptance diagnostics.
func Validate(f Fixture) error {
	canonical := Canonical()
	sourceJSON, err := json.Marshal(f.Sources)
	if err != nil {
		return errors.New("VERCEL-CONTRACT-SOURCES-BLOCKED")
	}
	if _, err := packsync.LoadConfig(bytes.NewReader(sourceJSON)); err != nil ||
		!reflect.DeepEqual(f.Sources, canonical.Sources) {
		return errors.New("VERCEL-CONTRACT-SOURCES-BLOCKED")
	}
	if _, err := capabilitypack.EncodePortableManifestV4(f.Pack); err != nil ||
		!reflect.DeepEqual(f.Pack, canonical.Pack) {
		return errors.New("VERCEL-CONTRACT-MANIFEST-BLOCKED")
	}
	if !reflect.DeepEqual(f.Legal, canonical.Legal) {
		return errors.New("VERCEL-CONTRACT-LEGAL-BLOCKED")
	}
	if !reflect.DeepEqual(f.Blobs, canonical.Blobs) ||
		!reflect.DeepEqual(f.Loaders, canonical.Loaders) ||
		!reflect.DeepEqual(f.Compatibility, canonical.Compatibility) ||
		!reflect.DeepEqual(f.Aliases, canonical.Aliases) ||
		f.SnapshotSHA256 != ExactArchiveSHA256 ||
		f.CatalogSelectable {
		return errors.New("VERCEL-CONTRACT-EVIDENCE-BLOCKED")
	}
	if _, err := InspectExactArchive(); err != nil {
		return errors.New("VERCEL-CONTRACT-SNAPSHOT-BLOCKED")
	}
	return nil
}

type modeRow struct {
	id, role                  string
	requirements, authorities string
	effects, fallback         string
}

func modes(resource string) []capabilitypack.RuntimeMode {
	rows := map[string][]modeRow{
		"vercel-composition-patterns": {{"guidance-edit", "primary", "", "guidance-edit", "guidance-edit", ""}}, "vercel-react-best-practices": {{"guidance-edit", "primary", "", "guidance-edit", "guidance-edit", ""}}, "vercel-react-native-skills": {{"guidance-edit", "primary", "", "guidance-edit", "guidance-edit", ""}}, "vercel-react-view-transitions": {{"guidance-edit", "primary", "", "guidance-edit", "guidance-edit", ""}},
		"vercel-web-design-guidelines": {{"local-review", "primary", "", "local-review", "", ""}}, "vercel-writing-guidelines": {{"local-review", "primary", "", "local-review", "", ""}},
		"vercel-deploy-to-vercel": {{"claimable-preview", "fallback_only", "claimable", "claimable-preview", "claimable-preview", ""}, {"cli-preview", "primary", "vercel-cli-linked", "cli-preview", "cli-preview", "claimable-preview"}, {"cli-production", "primary", "vercel-cli-linked", "cli-production", "cli-production", ""}, {"git-push-preview", "primary", "git-push", "git-push-preview", "git-push-preview", "claimable-preview"}, {"git-push-production", "primary", "git-push", "git-push-production", "git-push-production", ""}, {"link-cli-preview", "primary", "vercel-cli-authenticated", "link-cli-preview", "link-cli-preview", "claimable-preview"}, {"link-git-preview", "primary", "vercel-cli-git-authenticated", "link-git-preview", "link-git-preview", "claimable-preview"}, {"setup-link-preview", "primary", "setup-link", "setup-link-preview", "setup-link-preview", "claimable-preview"}},
		"vercel-cli-with-tokens":  {{"deploy-preview", "primary", "token-cli-linked", "token-cli-preview", "cli-preview", ""}, {"deploy-production", "primary", "token-cli-linked", "token-cli-production", "cli-production", ""}, {"domain-read", "primary", "token-cli-linked", "token-inspect", "", ""}, {"domain-write", "primary", "token-cli-linked", "token-domain-write", "token-domain-write", ""}, {"environment-read", "primary", "token-cli-linked", "token-inspect", "", ""}, {"environment-write", "primary", "token-cli-linked", "token-environment-write", "token-environment-write", ""}, {"git-push-preview", "primary", "token-git-push", "token-git-push-preview", "git-push-preview", ""}, {"git-push-production", "primary", "token-git-push", "token-git-push-production", "git-push-production", ""}, {"inspect", "primary", "token-cli", "token-inspect", "", ""}, {"link-project", "primary", "token-cli", "token-link", "token-link", ""}},
		"vercel-optimize":         {{"sequential-investigation", "fallback_only", "optimize", "optimize-sequential", "optimize", ""}, {"sequential-observability-plus", "fallback_only", "optimize-observability-plus", "optimize-sequential", "optimize", ""}, {"subagent-investigation", "primary", "optimize", "optimize-subagent", "optimize", "sequential-investigation"}, {"subagent-observability-plus", "primary", "optimize-observability-plus", "optimize-subagent", "optimize", "sequential-observability-plus"}}}
	out := []capabilitypack.RuntimeMode{}
	for _, row := range rows[resource] {
		fallback := capabilitypack.RuntimeFallback{Kind: capabilitypack.RuntimeFallbackNone}
		if row.fallback != "" {
			fallback = capabilitypack.RuntimeFallback{Kind: capabilitypack.RuntimeFallbackMode, Mode: row.fallback}
		}
		out = append(out, capabilitypack.RuntimeMode{ID: row.id, Role: capabilitypack.RuntimeModeRole(row.role), Requirements: reqProfile(row.requirements), Authorities: authorityProfile(row.authorities), Effects: effectProfile(row.effects), Fallback: fallback, OnUnavailable: capabilitypack.RuntimeFailBeforeEffects})
	}
	return out
}
func reqProfile(p string) []capabilitypack.RuntimeRequirement {
	m := map[string][]string{"git-push": {"tool:git", "authentication:git-provider"}, "vercel-cli-authenticated": {"tool:vercel-cli", "authentication:vercel"}, "vercel-cli-linked": {"tool:vercel-cli", "authentication:vercel", "project_link:vercel-project"}, "vercel-cli-git-authenticated": {"tool:git", "tool:vercel-cli", "authentication:git-provider", "authentication:vercel"}, "setup-link": {"tool:npm", "authentication:vercel-interactive"}, "claimable": {"tool:bash", "tool:basename", "tool:cat", "tool:curl", "tool:cut", "tool:find", "tool:grep", "tool:head", "tool:mkdir", "tool:mktemp", "tool:mv", "tool:rm", "tool:sed", "tool:sleep", "tool:tar", "tool:tr", "tool:wc"}, "token-cli": {"tool:vercel-cli", "authentication:vercel-token"}, "token-cli-linked": {"tool:vercel-cli", "authentication:vercel-token", "project_link:vercel-project"}, "token-git-push": {"tool:git", "tool:vercel-cli", "authentication:git-provider", "authentication:vercel-token", "project_link:vercel-project"}, "optimize": {"tool:node@>=20.0.0", "tool:vercel-cli@>=53.0.0", "authentication:vercel", "project_link:vercel-project", "service_data:vercel-project-metrics"}}
	if p == "optimize-observability-plus" {
		m[p] = append(append([]string{}, m["optimize"]...), "entitlement:observability-plus")
	}
	out := []capabilitypack.RuntimeRequirement{}
	for _, v := range m[p] {
		parts := strings.SplitN(v, ":", 2)
		id := parts[1]
		ver := ""
		if i := strings.Index(id, "@"); i >= 0 {
			ver = id[i+1:]
			id = id[:i]
		}
		out = append(out, capabilitypack.RuntimeRequirement{Kind: capabilitypack.RuntimeRequirementKind(parts[0]), ID: id, Version: ver})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out
}
func authorityProfile(p string) []capabilitypack.RuntimeAuthority {
	base := map[string][]string{"guidance-edit": {"filesystem_read:consumer_project", "filesystem_write:consumer_project"}, "local-review": {"filesystem_read:consumer_project", "filesystem_read:pack_resource"}, "git-push-preview": {"filesystem_read:consumer_project", "git_commit:local_git", "git_inspect:local_git", "git_push:remote_git", "network:remote_git", "preview_deploy:vercel_project", "process_execute:local_git"}, "cli-preview": {"filesystem_read:consumer_project", "network:vercel_project", "preview_deploy:vercel_project", "process_execute:consumer_project", "secret_use:vercel_account", "upload:deployment_payload"}, "claimable-preview": {"filesystem_read:consumer_project", "network:vercel_project", "preview_deploy:vercel_project", "process_execute:consumer_project", "upload:deployment_payload"}, "token-inspect": {"environment_inspect:consumer_project", "filesystem_read:consumer_project", "network:vercel_account", "process_execute:consumer_project", "secret_use:vercel_account"}, "optimize-sequential": {"environment_inspect:consumer_project", "filesystem_read:consumer_project", "filesystem_read:pack_resource", "filesystem_write:consumer_project", "network:vercel_project", "process_execute:consumer_project", "secret_use:vercel_account"}}
	clone := func(x string) []string { return append([]string{}, base[x]...) }
	replace := func(xs []string, a, b string) []string {
		for i := range xs {
			if strings.HasPrefix(xs[i], a+":") {
				xs[i] = b + strings.TrimPrefix(xs[i], a)
			}
		}
		return xs
	}
	base["git-push-production"] = replace(clone("git-push-preview"), "preview_deploy", "production_deploy")
	base["cli-production"] = replace(clone("cli-preview"), "preview_deploy", "production_deploy")
	base["link-cli-preview"] = append(clone("cli-preview"), "filesystem_write:consumer_project", "vercel_project_mutate:vercel_project")
	base["link-git-preview"] = append(clone("git-push-preview"), "filesystem_write:consumer_project", "network:vercel_project", "secret_use:vercel_account", "vercel_project_mutate:vercel_project")
	base["setup-link-preview"] = append(clone("link-cli-preview"), "package_manager_execute:workstation")
	base["token-cli-preview"] = clone("cli-preview")
	base["token-cli-production"] = clone("cli-production")
	base["token-link"] = append(clone("token-inspect"), "filesystem_write:consumer_project", "vercel_project_mutate:vercel_project")
	base["token-environment-write"] = append(clone("token-inspect"), "filesystem_write:consumer_project", "vercel_environment_mutate:vercel_project")
	base["token-domain-write"] = append(clone("token-inspect"), "vercel_domain_mutate:vercel_project")
	base["token-git-push-preview"] = append(clone("git-push-preview"), "environment_inspect:consumer_project", "network:vercel_account", "secret_use:vercel_account")
	base["token-git-push-production"] = replace(clone("token-git-push-preview"), "preview_deploy", "production_deploy")
	base["optimize-subagent"] = append(clone("optimize-sequential"), "subagent_delegate:consumer_project")
	out := []capabilitypack.RuntimeAuthority{}
	for _, v := range base[p] {
		x := strings.Split(v, ":")
		out = append(out, capabilitypack.RuntimeAuthority{Kind: capabilitypack.RuntimeAuthorityKind(x[0]), Scope: capabilitypack.RuntimeScope(x[1])})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Scope < out[j].Scope
	})
	return out
}
func effectProfile(p string) []capabilitypack.RuntimeEffect {
	m := map[string][]string{"guidance-edit": {"consumer_project_dependency_change:consumer_project", "consumer_project_file_change:consumer_project"}, "git-push-preview": {"local_git_change:local_git", "preview_deployment:vercel_project", "remote_git_change:remote_git"}, "git-push-production": {"local_git_change:local_git", "production_deployment:vercel_project", "remote_git_change:remote_git"}, "cli-preview": {"preview_deployment:vercel_project", "upload:deployment_payload"}, "cli-production": {"production_deployment:vercel_project", "upload:deployment_payload"}, "link-cli-preview": {"authentication_state_change:vercel_account", "consumer_project_file_change:consumer_project", "preview_deployment:vercel_project", "upload:deployment_payload", "vercel_project_change:vercel_project"}, "link-git-preview": {"consumer_project_file_change:consumer_project", "local_git_change:local_git", "preview_deployment:vercel_project", "remote_git_change:remote_git", "vercel_project_change:vercel_project"}, "setup-link-preview": {"authentication_state_change:vercel_account", "consumer_project_file_change:consumer_project", "preview_deployment:vercel_project", "tool_installation:workstation", "upload:deployment_payload", "vercel_project_change:vercel_project"}, "claimable-preview": {"preview_deployment:vercel_project", "upload:deployment_payload"}, "token-link": {"consumer_project_file_change:consumer_project", "vercel_project_change:vercel_project"}, "token-environment-write": {"consumer_project_file_change:consumer_project", "vercel_environment_change:vercel_project"}, "token-domain-write": {"vercel_domain_change:vercel_project"}, "optimize": {"consumer_project_file_change:consumer_project"}}
	out := []capabilitypack.RuntimeEffect{}
	for _, v := range m[p] {
		x := strings.Split(v, ":")
		out = append(out, capabilitypack.RuntimeEffect{Kind: capabilitypack.RuntimeEffectKind(x[0]), Scope: capabilitypack.RuntimeScope(x[1])})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Scope < out[j].Scope
	})
	return out
}
