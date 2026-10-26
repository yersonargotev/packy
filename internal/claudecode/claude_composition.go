package claudecode

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

var claudeReferenceIDs = []string{"accessibility-checklist", "definition-of-done", "observability-checklist", "orchestration-patterns", "performance-checklist", "security-checklist", "testing-patterns"}

type compositeFile struct {
	Path    string `json:"path"`
	Content []byte `json:"content"`
	Mode    uint32 `json:"mode"`
}

type compositeOwnership struct {
	PackID                string `json:"pack_id"`
	PackVersion           string `json:"pack_version"`
	PortableKind          string `json:"portable_kind"`
	PortableID            string `json:"portable_id"`
	EffectiveName         string `json:"effective_name"`
	TargetType            string `json:"target_type"`
	SourceFingerprint     string `json:"source_fingerprint"`
	TreeFingerprint       string `json:"tree_fingerprint"`
	DefinitionFingerprint string `json:"definition_fingerprint"`
}

type compositeSkill struct {
	Files                 []compositeFile
	TreeFingerprint       string
	SourceFingerprint     string
	DefinitionFingerprint string
	Ownership             compositeOwnership
}

type compositeSkillBuilder func(capabilitypack.Pack, capabilitypack.Resource, capabilitypack.Binding, string) (compositeSkill, error)

func claudeCompositeSkillBuilder(pack capabilitypack.Pack, resource capabilitypack.Resource, binding capabilitypack.Binding) compositeSkillBuilder {
	if binding.Surface != capabilitypack.SurfaceClaude || binding.Projection != "skill" {
		return nil
	}
	if _, ok := resource.SurfaceCapability(capabilitypack.SurfaceClaude, capabilitypack.SurfaceCapabilityClaudeCompositeSkill); ok && resource.Kind == "skill" {
		return composeClaudeSkill
	}
	return nil
}

func claudeCompositeSkill(pack capabilitypack.Pack, resource capabilitypack.Resource, binding capabilitypack.Binding, catalogRoot string) (compositeSkill, error) {
	builder := claudeCompositeSkillBuilder(pack, resource, binding)
	if builder == nil {
		return compositeSkill{}, errors.New("resource has no Claude composite skill translation")
	}
	return builder(pack, resource, binding, catalogRoot)
}

// composeClaudeSkill is a pure translation: it reads selected Pack bytes but
// never interprets or executes skill content.
func composeClaudeSkill(pack capabilitypack.Pack, resource capabilitypack.Resource, binding capabilitypack.Binding, catalogRoot string) (compositeSkill, error) {
	capability, ok := resource.SurfaceCapability(capabilitypack.SurfaceClaude, capabilitypack.SurfaceCapabilityClaudeCompositeSkill)
	if !ok || capability.ClaudeCompositeSkill == nil {
		return compositeSkill{}, errors.New("resource does not declare Claude composite skill capability")
	}
	if resource.Kind != "skill" {
		return compositeSkill{}, fmt.Errorf("unsupported Claude composite kind %q", resource.Kind)
	}
	if binding.Surface != capabilitypack.SurfaceClaude || binding.Projection != "skill" || binding.Name == "" {
		return compositeSkill{}, errors.New("invalid Claude Claude skill binding")
	}
	source, err := safeCatalogPath(resource.CatalogRootOr(catalogRoot), resource.Source)
	if err != nil {
		return compositeSkill{}, err
	}
	files, err := readCompositeTree(source)
	if err != nil {
		return compositeSkill{}, err
	}
	for _, reference := range capability.ClaudeCompositeSkill.References {
		asset, err := uniqueResource(pack, reference.Kind, reference.ID)
		if err != nil {
			return compositeSkill{}, err
		}
		path, err := safeCatalogPath(asset.CatalogRootOr(catalogRoot), asset.Source)
		if err != nil {
			return compositeSkill{}, err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return compositeSkill{}, fmt.Errorf("read Claude composite reference %s:%s: %w", reference.Kind, reference.ID, err)
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return compositeSkill{}, fmt.Errorf("Claude composite reference %s:%s is not a regular file", reference.Kind, reference.ID)
		}
		files = append(files, compositeFile{Path: "references/" + filepath.Base(asset.Source), Content: content, Mode: normalizedMode(info.Mode())})
	}
	if err := canonicalizeCompositeFiles(files); err != nil {
		return compositeSkill{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	sourceFP, err := sourceFingerprint(source)
	if err != nil {
		return compositeSkill{}, err
	}
	definition, err := json.Marshal(struct {
		PackID, Version string
		Resource        capabilitypack.Resource
		Binding         capabilitypack.Binding
	}{pack.ID, pack.Version, resource, binding})
	if err != nil {
		return compositeSkill{}, err
	}
	treeFP, err := fingerprintCompositeFiles(files, 0o700)
	if err != nil {
		return compositeSkill{}, err
	}
	result := compositeSkill{Files: files, TreeFingerprint: treeFP, SourceFingerprint: sourceFP, DefinitionFingerprint: localprojection.FingerprintBytes(definition)}
	result.Ownership = compositeOwnership{PackID: pack.ID, PackVersion: pack.Version, PortableKind: resource.Kind, PortableID: resource.ID, EffectiveName: binding.Name, TargetType: "claude-personal-skill", SourceFingerprint: result.SourceFingerprint, TreeFingerprint: result.TreeFingerprint, DefinitionFingerprint: result.DefinitionFingerprint}
	return result, nil
}

func safeCatalogPath(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) || filepath.Clean(relative) != relative || relative == "." || strings.HasPrefix(filepath.ToSlash(relative), "../") {
		return "", fmt.Errorf("unsafe Claude source path %q", relative)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve Claude catalog root: %w", err)
	}
	candidate := filepath.Join(root, filepath.FromSlash(relative))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	within, err := filepath.Rel(canonicalRoot, resolved)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) || filepath.IsAbs(within) {
		return "", fmt.Errorf("Claude source escapes catalog root %q", relative)
	}
	return candidate, nil
}

func readCompositeTree(root string) ([]compositeFile, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("Claude skill source is not a directory")
	}
	var files []compositeFile
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("Claude skill source contains symlink %q", path)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Claude skill source contains non-regular file %q", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, compositeFile{Path: filepath.ToSlash(rel), Content: content, Mode: normalizedMode(info.Mode())})
		return nil
	})
	return files, err
}

func normalizedMode(mode os.FileMode) uint32 {
	if mode&0o111 != 0 {
		return 0o755
	}
	return 0o644
}

func sourceFingerprint(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return localprojection.FingerprintExactTree(path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return localprojection.FingerprintBytes(b), nil
}

func uniqueResource(pack capabilitypack.Pack, kind, id string) (capabilitypack.Resource, error) {
	var found *capabilitypack.Resource
	for i := range pack.Resources {
		if pack.Resources[i].Kind == kind && pack.Resources[i].ID == id {
			if found != nil {
				return capabilitypack.Resource{}, fmt.Errorf("duplicate Claude dependency %s:%s", kind, id)
			}
			r := pack.Resources[i]
			found = &r
		}
	}
	if found == nil {
		return capabilitypack.Resource{}, fmt.Errorf("missing Claude dependency %s:%s", kind, id)
	}
	return *found, nil
}

func yamlScalar(value string) string { encoded, _ := json.Marshal(value); return string(encoded) }

func canonicalizeCompositeFiles(files []compositeFile) error {
	seen := map[string]bool{}
	for _, file := range files {
		if file.Path == "" || file.Path != filepath.ToSlash(filepath.Clean(file.Path)) || strings.HasPrefix(file.Path, "/") || strings.HasPrefix(file.Path, "../") {
			return fmt.Errorf("noncanonical composite path %q", file.Path)
		}
		if seen[file.Path] {
			return fmt.Errorf("duplicate composite path %q", file.Path)
		}
		seen[file.Path] = true
		if file.Mode != 0o644 && file.Mode != 0o755 {
			return fmt.Errorf("invalid composite mode %o", file.Mode)
		}
	}
	return nil
}

func fingerprintCompositeFiles(files []compositeFile, directoryMode fs.FileMode) (string, error) {
	treeFiles := make([]localprojection.TreeFile, len(files))
	for i, file := range files {
		treeFiles[i] = localprojection.TreeFile{Path: file.Path, Content: file.Content, Mode: fs.FileMode(file.Mode)}
	}
	return localprojection.FingerprintTreeFiles(treeFiles, directoryMode)
}

type compositeSkillPayload struct {
	Files                 []compositeFile    `json:"files"`
	TreeFingerprint       string             `json:"tree_fingerprint"`
	SourceFingerprint     string             `json:"source_fingerprint"`
	DefinitionFingerprint string             `json:"definition_fingerprint"`
	Ownership             compositeOwnership `json:"ownership"`
}

func canonicalCompositeSkillPayload(skill compositeSkill) ([]byte, error) {
	p := compositeSkillPayload{skill.Files, skill.TreeFingerprint, skill.SourceFingerprint, skill.DefinitionFingerprint, skill.Ownership}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func canonicalCompositeOwnership(ownership compositeOwnership) (string, error) {
	data, err := json.Marshal(ownership)
	return string(data), err
}

func decodeCompositeOwnership(data string) (compositeOwnership, error) {
	var ownership compositeOwnership
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ownership); err != nil {
		return compositeOwnership{}, fmt.Errorf("decode composite ownership: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return compositeOwnership{}, errors.New("trailing composite ownership data")
	}
	canonical, err := canonicalCompositeOwnership(ownership)
	if err != nil || canonical != data {
		return compositeOwnership{}, errors.New("noncanonical composite ownership")
	}
	if ownership.PackID == "" || ownership.PackVersion == "" || ownership.PortableKind != "skill" || ownership.PortableID == "" || ownership.EffectiveName == "" || ownership.TargetType != "claude-personal-skill" {
		return compositeOwnership{}, errors.New("incomplete composite ownership")
	}
	for _, fingerprint := range []string{ownership.SourceFingerprint, ownership.TreeFingerprint, ownership.DefinitionFingerprint} {
		if len(fingerprint) != 64 {
			return compositeOwnership{}, errors.New("invalid composite ownership fingerprint")
		}
		if _, err := hex.DecodeString(fingerprint); err != nil {
			return compositeOwnership{}, errors.New("invalid composite ownership fingerprint")
		}
	}
	return ownership, nil
}

func compositeTreeFiles(skill compositeSkill) []localprojection.TreeFile {
	files := make([]localprojection.TreeFile, len(skill.Files))
	for i, file := range skill.Files {
		files[i] = localprojection.TreeFile{Path: file.Path, Content: append([]byte(nil), file.Content...), Mode: fs.FileMode(file.Mode)}
	}
	return files
}

func decodeCompositeSkillPayload(data []byte) (compositeSkill, error) {
	var p compositeSkillPayload
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return compositeSkill{}, fmt.Errorf("decode composite skill payload: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return compositeSkill{}, errors.New("trailing composite skill payload")
	}
	canonical, err := json.Marshal(p)
	if err != nil || !bytes.Equal(canonical, data) {
		return compositeSkill{}, errors.New("noncanonical composite skill payload")
	}
	if err := canonicalizeCompositeFiles(p.Files); err != nil {
		return compositeSkill{}, err
	}
	if !sort.SliceIsSorted(p.Files, func(i, j int) bool { return p.Files[i].Path < p.Files[j].Path }) {
		return compositeSkill{}, errors.New("unsorted composite skill payload")
	}
	fingerprint, err := fingerprintCompositeFiles(p.Files, 0o700)
	if err != nil || fingerprint != p.TreeFingerprint {
		return compositeSkill{}, errors.New("composite skill tree fingerprint mismatch")
	}
	if p.Ownership.TreeFingerprint != p.TreeFingerprint || p.Ownership.SourceFingerprint != p.SourceFingerprint || p.Ownership.DefinitionFingerprint != p.DefinitionFingerprint {
		return compositeSkill{}, errors.New("composite skill ownership fingerprint mismatch")
	}
	if _, err := decodeCompositeOwnership(mustCompositeOwnership(p.Ownership)); err != nil {
		return compositeSkill{}, err
	}
	return compositeSkill{p.Files, p.TreeFingerprint, p.SourceFingerprint, p.DefinitionFingerprint, p.Ownership}, nil
}

func mustCompositeOwnership(ownership compositeOwnership) string {
	data, _ := canonicalCompositeOwnership(ownership)
	return data
}
