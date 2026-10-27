package capabilitypack

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yersonargotev/packy/internal/skilltemplate"
	"go.yaml.in/yaml/v3"
)

// ExportedResource describes the selected native closure, not runtime readiness.
// Roles have no package path: their definitions are never registered or copied.
type ExportedResource struct {
	Resource    ResourceIdentity `json:"resource"`
	Source      string           `json:"source"`
	Path        string           `json:"path,omitempty"`
	Origin      *ResourceOrigin  `json:"origin,omitempty"`
	License     string           `json:"license,omitempty"`
	Attribution string           `json:"attribution,omitempty"`
	Roles       []PluginRole     `json:"roles,omitempty"`
}

type PluginExportResult struct {
	SnapshotID      string             `json:"snapshot_id"`
	PackID          string             `json:"pack_id"`
	PackVersion     string             `json:"pack_version"`
	Surface         Surface            `json:"surface"`
	Format          string             `json:"format"`
	SelectedSkills  []string           `json:"selected_skills"`
	Resources       []ExportedResource `json:"resources"`
	PluginName      string             `json:"plugin_name"`
	OutputDirectory string             `json:"output_directory"`
	ContentSHA256   string             `json:"content_sha256"`
	Prerequisites   []string           `json:"prerequisites"`
}

type pluginFile struct {
	data []byte
	mode fs.FileMode
}
type pluginPlan struct {
	result PluginExportResult
	files  map[string]pluginFile
}

var retainedPluginSnapshot = regexp.MustCompile(`^[0-9a-f]{40}$`)

// ExportPlugin consumes an already validated immutable snapshot supplied by the
// retained catalog boundary. All planning and validation precede publication.
func ExportPlugin(ctx context.Context, snapshotRoot, snapshotID, packID string, surface Surface, skills []string, output string) (PluginExportResult, error) {
	if !idPattern.MatchString(packID) || !retainedPluginSnapshot.MatchString(snapshotID) {
		return PluginExportResult{}, fmt.Errorf("export requires a Pack ID and retained immutable Catalog Snapshot identity")
	}
	if surface != SurfaceAgents && surface != SurfaceClaude {
		return PluginExportResult{}, fmt.Errorf("plugin export supports agents and claude; received %q", surface)
	}
	pack, err := ValidatePackContent(snapshotRoot, packID)
	if err != nil {
		return PluginExportResult{}, err
	}
	if !pack.Selectable || !slices.Contains(pack.Surfaces, surface) {
		return PluginExportResult{}, fmt.Errorf("Pack %s is not selectable on %s", packID, surface)
	}
	plan, err := planPluginExport(ctx, pack, snapshotID, surface, skills)
	if err != nil {
		return PluginExportResult{}, err
	}
	destination, err := publishPlugin(ctx, snapshotRoot, output, plan.files)
	if err != nil {
		return PluginExportResult{}, err
	}
	plan.result.OutputDirectory = destination
	return plan.result, nil
}

func planPluginExport(ctx context.Context, pack Pack, snapshotID string, surface Surface, skills []string) (pluginPlan, error) {
	format := AgentPluginsFormat
	if surface == SurfaceClaude {
		format = ClaudePluginFormat
	}
	pack = withSurfaceCapabilityDependencies(ResolvePackForSurface(pack, surface), surface)
	roots := []ResourceIdentity{}
	if len(skills) == 0 {
		for _, resource := range pack.Resources {
			if resource.Kind == "skill" && bindingForSurface(resource, surface) != nil && pluginContract(resource, format) != nil {
				roots = append(roots, ResourceIdentity{Kind: "skill", ID: resource.ID})
			}
		}
	} else {
		for _, id := range skills {
			if !idPattern.MatchString(id) {
				return pluginPlan{}, fmt.Errorf("export skill %q must be a logical skill ID within Pack %s", id, pack.ID)
			}
			roots = append(roots, ResourceIdentity{Kind: "skill", ID: id})
		}
	}
	if len(roots) == 0 {
		return pluginPlan{}, fmt.Errorf("Pack %s has no reviewed %s eligible skills", pack.ID, format)
	}
	selection, err := canonicalSelection(ResourceSelection{Mode: SelectionCustom, Roots: roots})
	if err != nil {
		return pluginPlan{}, err
	}
	resources, _, err := resolveResourceClosure(pack, selection.Roots)
	if err != nil {
		return pluginPlan{}, err
	}
	// Legal references are part of the closure even when a notice itself carries
	// another reviewed notice. The native operational dependency graph stays intact.
	selected := map[string]bool{}
	for _, r := range resources {
		selected[r.Kind+":"+r.ID] = true
	}
	for i := 0; i < len(resources); i++ {
		for _, notice := range resources[i].Notices {
			if selected[notice] {
				continue
			}
			found := false
			for _, r := range pack.Resources {
				if r.Kind+":"+r.ID == notice {
					resources = append(resources, r)
					selected[notice] = true
					found = true
					break
				}
			}
			if !found {
				return pluginPlan{}, fmt.Errorf("missing legal resource %s", notice)
			}
		}
	}
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Kind+":"+resources[i].ID < resources[j].Kind+":"+resources[j].ID
	})
	result := PluginExportResult{SnapshotID: snapshotID, PackID: pack.ID, PackVersion: pack.Version, Surface: surface, Format: format, PluginName: pack.ID, SelectedSkills: []string{}, Resources: []ExportedResource{}, Prerequisites: slices.Clone(pack.Requires.Tools)}
	for _, root := range selection.Roots {
		result.SelectedSkills = append(result.SelectedSkills, root.ID)
	}
	targets := map[string]string{}
	names := map[string]string{}
	prereqs := map[string]bool{}
	for _, p := range result.Prerequisites {
		prereqs[p] = true
	}
	for _, resource := range resources {
		identity := resource.Kind + ":" + resource.ID
		switch resource.Kind {
		case "skill":
			contract := pluginContract(resource, format)
			if bindingForSurface(resource, surface) == nil || contract == nil {
				return pluginPlan{}, fmt.Errorf("required skill %s is not reviewed for %s on %s", identity, format, surface)
			}
			for _, existing := range names {
				if existing == contract.Name {
					return pluginPlan{}, fmt.Errorf("duplicate plugin skill name %s", existing)
				}
			}
			names[identity] = contract.Name
			targets[identity] = path.Join("skills", contract.Name)
			for _, p := range contract.Prerequisites {
				prereqs[p] = true
			}
			for _, dep := range resource.Requires {
				if strings.HasPrefix(dep, "agent:") {
					resolved := false
					for _, role := range contract.Roles {
						if role.Resource == dep {
							resolved = true
							if role.Resolution == "external" {
								prereqs[dep+": "+role.Description] = true
							}
						}
					}
					if !resolved {
						return pluginPlan{}, fmt.Errorf("skill %s requires a reviewed plugin adaptation or external prerequisite for %s", identity, dep)
					}
				}
			}
		case "asset", "notice":
			targets[identity] = resource.Source
		case "agent":
		// Its native transitive dependencies and notices remain in the closure.
		default:
			return pluginPlan{}, fmt.Errorf("plugin closure cannot contain %s", identity)
		}
		if resource.Kind != "skill" {
			for _, dep := range resource.Requires {
				if strings.HasPrefix(dep, "agent:") {
					return pluginPlan{}, fmt.Errorf("required role %s through %s has no reviewed plugin resolution", dep, identity)
				}
			}
		}
	}
	files := map[string]pluginFile{}
	for _, resource := range resources {
		if err := ctx.Err(); err != nil {
			return pluginPlan{}, err
		}
		identity := resource.Kind + ":" + resource.ID
		source := resource.Source
		origin := resource.Origin
		var roles []PluginRole
		if resource.Kind == "skill" {
			contract := pluginContract(resource, format)
			roles = contract.Roles
			if contract.Source != "" {
				source = contract.Source
				origin = contract.Origin
			}
		}
		result.Resources = append(result.Resources, ExportedResource{Resource: ResourceIdentity{Kind: resource.Kind, ID: resource.ID}, Source: source, Path: targets[identity], Origin: origin, License: resource.License, Attribution: resource.Attribution, Roles: roles})
		if resource.Kind == "agent" {
			continue
		}
		root, err := os.OpenRoot(resource.CatalogRootOr(""))
		if err != nil {
			return pluginPlan{}, err
		}
		copied, err := pluginSourceFiles(ctx, root, source, targets[identity], resource.Kind == "skill")
		root.Close()
		if err != nil {
			return pluginPlan{}, fmt.Errorf("export %s: %w", identity, err)
		}
		if resource.Kind == "skill" {
			contract := pluginContract(resource, format)
			if err := renderPluginReferences(resource, contract, copied, targets, names); err != nil {
				return pluginPlan{}, fmt.Errorf("export %s: %w", identity, err)
			}
			entry := path.Join(targets[identity], "SKILL.md")
			file, ok := copied[entry]
			if !ok {
				return pluginPlan{}, fmt.Errorf("skill %s is missing SKILL.md", identity)
			}
			file.data, err = skilltemplate.RenameFrontmatterName(file.data, contract.Name)
			if err != nil {
				return pluginPlan{}, err
			}
			if err := validatePluginSkill(file.data, contract.Name, format); err != nil {
				return pluginPlan{}, fmt.Errorf("skill %s: %w", identity, err)
			}
			copied[entry] = file
		}
		for target, file := range copied {
			if _, exists := files[target]; exists {
				return pluginPlan{}, fmt.Errorf("plugin output path collision at %s", target)
			}
			files[target] = file
		}
	}
	result.Prerequisites = []string{}
	for p := range prereqs {
		result.Prerequisites = append(result.Prerequisites, p)
	}
	slices.Sort(result.Prerequisites)
	manifest := struct {
		Schema      string `json:"$schema"`
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
	}{"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json", pack.ID, pack.Version, pack.Description}
	if len(pack.ID) > 64 {
		return pluginPlan{}, fmt.Errorf("plugin Pack name exceeds 64 characters")
	}
	manifestPath := "plugin.json"
	var data []byte
	if format == ClaudePluginFormat {
		manifestPath = ".claude-plugin/plugin.json"
		data, _ = json.MarshalIndent(struct {
			Name        string `json:"name"`
			Version     string `json:"version"`
			Description string `json:"description"`
		}{pack.ID, pack.Version, pack.Description}, "", "  ")
	} else {
		data, _ = json.MarshalIndent(manifest, "", "  ")
	}
	if _, exists := files[manifestPath]; exists {
		return pluginPlan{}, fmt.Errorf("plugin output path collision at %s", manifestPath)
	}
	if _, exists := files["packy-provenance.json"]; exists {
		return pluginPlan{}, fmt.Errorf("plugin output path collision at packy-provenance.json")
	}
	files[manifestPath] = pluginFile{append(data, '\n'), 0644}
	origins, err := selectedPluginOrigins(pack, result.Resources)
	if err != nil {
		return pluginPlan{}, err
	}
	provenance := struct {
		SnapshotID  string             `json:"snapshot_id"`
		Origins     []PackOrigin       `json:"origins"`
		PackID      string             `json:"pack_id"`
		PackVersion string             `json:"pack_version"`
		Resources   []ExportedResource `json:"resources"`
	}{snapshotID, origins, pack.ID, pack.Version, result.Resources}
	data, err = json.MarshalIndent(provenance, "", "  ")
	if err != nil {
		return pluginPlan{}, err
	}
	files["packy-provenance.json"] = pluginFile{append(data, '\n'), 0644}
	if format == ClaudePluginFormat {
		for file := range files {
			for _, reserved := range []string{"agents/", "commands/", "hooks/", ".mcp.json", ".lsp.json", "settings.json"} {
				if file == reserved || strings.HasPrefix(file, reserved) {
					return pluginPlan{}, fmt.Errorf("Claude plugin contains unsupported runtime component %s", file)
				}
			}
		}
	}
	if err := validatePluginFiles(files); err != nil {
		return pluginPlan{}, err
	}
	result.ContentSHA256 = pluginFilesDigest(files)
	return pluginPlan{result: result, files: files}, nil
}

func pluginSourceFiles(ctx context.Context, root *os.Root, source, target string, tree bool) (map[string]pluginFile, error) {
	result := map[string]pluginFile{}
	if !validPluginPath(source) || !validPluginPath(target) {
		return nil, fmt.Errorf("unsafe plugin source or destination path")
	}
	// Lstat every ancestor as well as leaves: no linked directory can import
	// another resource or escape the declared source tree.
	current := ""
	for _, component := range strings.Split(source, "/") {
		current = path.Join(current, component)
		info, err := root.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("linked source %s is unsupported", current)
		}
	}
	read := func(file, destination string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := root.Lstat(file)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular source %s", file)
		}
		data, err := root.ReadFile(file)
		if err != nil {
			return err
		}
		if info.Mode().Perm() != 0644 && info.Mode().Perm() != 0755 {
			return fmt.Errorf("unsupported source mode for %s", file)
		}
		result[destination] = pluginFile{data, info.Mode().Perm()}
		return nil
	}
	if !tree {
		return result, read(source, target)
	}
	err := fs.WalkDir(root.FS(), source, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(filepath.FromSlash(source), filepath.FromSlash(file))
		if err != nil {
			return err
		}
		return read(file, path.Join(target, filepath.ToSlash(rel)))
	})
	return result, err
}

func renderPluginReferences(resource Resource, contract *PluginExport, files map[string]pluginFile, targets, names map[string]string) error {
	base := targets["skill:"+resource.ID]
	for _, ref := range contract.References {
		filePath := path.Join(base, ref.File)
		file, ok := files[filePath]
		if !ok {
			return fmt.Errorf("plugin reference %s file %s is missing", ref.ID, ref.File)
		}
		marker := "{{packy.export:" + ref.ID + "}}"
		if !bytes.Contains(file.data, []byte(marker)) {
			return fmt.Errorf("plugin reference %s has no exact authored marker", ref.ID)
		}
		value := ""
		switch ref.Kind {
		case "name":
			value = names[ref.Target]
		case "invocation":
			if names[ref.Target] != "" {
				value = ref.Value
			}
		case "path":
			if target := targets[ref.Target]; target != "" {
				relative, err := filepath.Rel(filepath.FromSlash(path.Dir(filePath)), filepath.FromSlash(target))
				if err != nil {
					return err
				}
				value = filepath.ToSlash(relative)
			}
		}
		if value == "" {
			return fmt.Errorf("plugin reference %s target %s is not packaged", ref.ID, ref.Target)
		}
		file.data = bytes.ReplaceAll(file.data, []byte(marker), []byte(value))
		files[filePath] = file
	}
	return nil
}

func validatePluginSkill(data []byte, name, format string) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("SKILL.md must be UTF-8")
	}
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return fmt.Errorf("SKILL.md requires YAML frontmatter")
	}
	end := bytes.Index(normalized[4:], []byte("\n---\n"))
	if end < 0 {
		return fmt.Errorf("SKILL.md requires a closing frontmatter delimiter")
	}
	var header map[string]any
	if err := yaml.Unmarshal(normalized[4:4+end], &header); err != nil {
		return err
	}
	for field := range header {
		switch field {
		case "name", "description", "license", "compatibility", "metadata", "allowed-tools":
		case "when_to_use", "argument-hint", "arguments", "disable-model-invocation", "user-invocable", "disallowed-tools", "model", "effort", "context", "agent", "background", "paths":
			if format != ClaudePluginFormat {
				return fmt.Errorf("unsupported Agent Skills frontmatter field %q; declare a reviewed plugin adaptation", field)
			}
		default:
			return fmt.Errorf("unsupported Agent Skills frontmatter field %q; declare a reviewed plugin adaptation", field)
		}
	}
	if header["name"] != name {
		return fmt.Errorf("skill name does not match plugin directory")
	}
	description, ok := header["description"].(string)
	if !ok || strings.TrimSpace(description) == "" || utf8.RuneCountInString(description) > 1024 {
		return fmt.Errorf("skill description must be a nonempty string of at most 1024 characters")
	}
	for _, field := range []string{"license", "compatibility", "allowed-tools"} {
		if value, exists := header[field]; exists {
			text, ok := value.(string)
			if !ok && !(field == "allowed-tools" && format == ClaudePluginFormat && pluginStringList(value)) || field == "compatibility" && (text == "" || utf8.RuneCountInString(text) > 500) {
				return fmt.Errorf("invalid skill %s metadata", field)
			}
		}
	}
	if format == ClaudePluginFormat {
		if value, ok := header["context"]; ok && fmt.Sprint(value) != "fork" {
			return fmt.Errorf("Claude skill context must be fork")
		}
		if _, ok := header["background"]; ok && fmt.Sprint(header["context"]) != "fork" {
			return fmt.Errorf("Claude skill background requires context: fork")
		}
		if value, ok := header["effort"]; ok && !slices.Contains([]string{"low", "medium", "high", "xhigh", "max"}, fmt.Sprint(value)) {
			return fmt.Errorf("invalid Claude skill effort")
		}
		for _, field := range []string{"when_to_use", "argument-hint", "model", "effort", "context", "agent"} {
			if value, exists := header[field]; exists {
				if _, ok := value.(string); !ok {
					return fmt.Errorf("invalid skill %s metadata", field)
				}
			}
		}
		for _, field := range []string{"disable-model-invocation", "user-invocable", "background"} {
			if value, exists := header[field]; exists {
				if _, ok := value.(bool); !ok {
					return fmt.Errorf("invalid skill %s metadata", field)
				}
			}
		}
		for _, field := range []string{"arguments", "disallowed-tools", "paths"} {
			if value, exists := header[field]; exists {
				if _, ok := value.(string); !ok && !pluginStringList(value) {
					return fmt.Errorf("invalid skill %s metadata", field)
				}
			}
		}
	}
	if value, exists := header["metadata"]; exists {
		mapping, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("skill metadata must be a string mapping")
		}
		for _, v := range mapping {
			if _, ok := v.(string); !ok {
				return fmt.Errorf("skill metadata must be a string mapping")
			}
		}
	}
	return nil
}

func pluginStringList(value any) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if _, ok := item.(string); !ok {
			return false
		}
	}
	return true
}

func validatePluginFiles(files map[string]pluginFile) error {
	for file, content := range files {
		if !validPluginPath(file) || strings.Contains("/"+strings.ToLower(file)+"/", "/.git/") {
			return fmt.Errorf("unsafe plugin output path %s", file)
		}
		// Do not infer or rewrite prose. Unrendered authored Packy markers are invalid
		// rather than silently producing a package dependent on catalog preparation.
		if bytes.Contains(content.data, []byte("{{packy.")) {
			return fmt.Errorf("unresolved Packy reference in %s", file)
		}
		for parent := path.Dir(file); parent != "."; parent = path.Dir(parent) {
			if _, ok := files[parent]; ok {
				return fmt.Errorf("plugin file/directory collision at %s", parent)
			}
		}
	}
	return nil
}

func pluginFilesDigest(files map[string]pluginFile) string {
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	hash := sha256.New()
	for _, key := range keys {
		file := files[key]
		fmt.Fprintf(hash, "%s\x00%o\x00%d\x00", key, file.mode, len(file.data))
		hash.Write(file.data)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func selectedPluginOrigins(pack Pack, resources []ExportedResource) ([]PackOrigin, error) {
	required := map[string]bool{}
	for _, resource := range resources {
		if resource.Origin != nil {
			required[resource.Origin.ID] = true
		}
	}
	result := []PackOrigin{}
	for _, origin := range pack.Origins {
		if required[origin.ID] {
			result = append(result, origin)
			delete(required, origin.ID)
		}
	}
	if len(required) > 0 {
		missing := []string{}
		for id := range required {
			missing = append(missing, id)
		}
		slices.Sort(missing)
		return nil, fmt.Errorf("plugin provenance is missing reviewed origins %v", missing)
	}
	slices.SortFunc(result, func(a, b PackOrigin) int { return strings.Compare(a.ID, b.ID) })
	return result, nil
}
