// Package skilldiscovery observes native and compatibility skill discovery.
// It does not own files, install launch settings, or grant runtime authority.
package skilldiscovery

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
	"go.yaml.in/yaml/v3"
)

// Observer supplies filesystem discovery facts to the domain inspection gateway.
// GlobalRoots and WorkingDirectory are explicit composition-root inputs.
type Observer struct {
	Surface          capabilitypack.Surface
	GlobalRoots      map[capabilitypack.Surface]string
	WorkingDirectory string
}

type tree struct {
	surface                 capabilitypack.Surface
	path, name, fingerprint string
}

func (a Observer) Observe(_ context.Context, transition capabilitypack.SurfaceTransition, result capabilitypack.SurfaceInspection) ([]capabilitypack.SkillDiscoveryFact, error) {
	var desired []tree
	for _, p := range result.Projections {
		if p.Goal == capabilitypack.ProjectionAbsent || (!strings.HasPrefix(p.ID, "skill:") && filepath.Base(filepath.Dir(p.Action.Target)) != "skills") {
			continue
		}
		root := p.Action.Source
		if root == "" && (transition.ReceiptOwnership != nil || transition.ProjectInstallation != nil || transition.ObservationOnly) {
			root = p.Action.Target
		}
		name, fingerprint, err := readTree(root)
		if len(p.Action.TreeFiles) > 0 {
			files := make([]localprojection.TreeFile, 0, len(p.Action.TreeFiles))
			var nameErr error
			for _, file := range p.Action.TreeFiles {
				files = append(files, localprojection.TreeFile{Path: file.Path, Content: file.Content, Mode: fs.FileMode(file.Mode)})
				if file.Path == "SKILL.md" {
					name, nameErr = skillName(file.Content, filepath.Base(p.Action.Target))
				}
			}
			fingerprint, err = localprojection.FingerprintTreeFiles(files)
			if nameErr != nil {
				err = nameErr
			}
		}
		if err != nil {
			// Generated composite trees do not exist until application. Their native
			// binding name still participates; unknown content cannot prove equality.
			name, fingerprint = filepath.Base(p.Action.Target), "unverified"
		}
		desired = append(desired, tree{a.Surface, p.Action.Target, name, fingerprint})
	}
	if len(desired) == 0 {
		return nil, nil
	}
	roots := []tree{}
	for surface, root := range a.GlobalRoots {
		if root != "" {
			roots = append(roots, tree{surface: surface, path: root})
			if surface == capabilitypack.SurfaceOpenCode {
				roots = append(roots, tree{surface: surface, path: filepath.Join(filepath.Dir(root), "skill")})
			}
		}
	}
	cwd := transition.ProjectRoot
	if cwd == "" {
		cwd = a.WorkingDirectory
	}
	for cwd != "" && cwd != "." {
		for surface, dir := range map[capabilitypack.Surface]string{capabilitypack.SurfaceCodex: ".agents", capabilitypack.SurfaceClaude: ".claude", capabilitypack.SurfaceOpenCode: ".opencode"} {
			roots = append(roots, tree{surface: surface, path: filepath.Join(cwd, dir, "skills")})
			if surface == capabilitypack.SurfaceOpenCode {
				roots = append(roots, tree{surface: surface, path: filepath.Join(cwd, dir, "skill")})
			}
		}
		if _, err := os.Stat(filepath.Join(cwd, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].path < roots[j].path })
	excluded := map[string]bool{}
	for _, d := range desired {
		excluded[targetKey(d.path)] = true
	}
	var facts []capabilitypack.SkillDiscoveryFact
	for i, left := range desired {
		for _, right := range desired[i+1:] {
			if left.name == right.name && targetKey(left.path) != targetKey(right.path) {
				facts = append(facts, capabilitypack.SkillDiscoveryFact{Host: a.Surface, Name: left.name, Target: left.path, OtherTarget: right.path, Fingerprint: left.fingerprint, OtherFingerprint: right.fingerprint})
			}
		}
	}
	for _, root := range roots {
		// Codex and Claude do not discover each other's roots. OpenCode discovers
		// both; either installation order can change what OpenCode will load.
		if a.Surface != capabilitypack.SurfaceOpenCode && root.surface != capabilitypack.SurfaceOpenCode && root.surface != a.Surface {
			continue
		}
		candidates, err := scan(root.path, map[string]bool{}, excluded)
		if err != nil {
			return nil, fmt.Errorf("inspect skill discovery at %s: %w", root.path, err)
		}
		for _, candidate := range candidates {
			for _, d := range desired {
				if filepath.Clean(candidate.path) == filepath.Clean(d.path) || candidate.name != d.name {
					continue
				}
				host := capabilitypack.SurfaceOpenCode
				if root.surface == a.Surface {
					host = a.Surface
				}
				facts = append(facts, capabilitypack.SkillDiscoveryFact{Host: host, Name: d.name, Target: d.path, OtherTarget: candidate.path, Fingerprint: d.fingerprint, OtherFingerprint: candidate.fingerprint})
			}
		}
	}
	return facts, nil
}

func scan(root string, visited, excluded map[string]bool) ([]tree, error) {
	if excluded[targetKey(root)] {
		return nil, nil
	}
	resolved, err := filepath.EvalSymlinks(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if visited[resolved] {
		return nil, nil
	}
	visited[resolved] = true
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}
	if _, err := os.Stat(filepath.Join(root, "SKILL.md")); err == nil {
		name, fp, err := readTree(root)
		if err != nil {
			name, fp = filepath.Base(root), "unverified"
		}
		return []tree{{path: root, name: name, fingerprint: fp}}, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var result []tree
	for _, entry := range entries {
		if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		values, err := scan(filepath.Join(root, entry.Name()), visited, excluded)
		if err != nil {
			return nil, err
		}
		result = append(result, values...)
	}
	return result, nil
}

func readTree(root string) (string, string, error) {
	if root == "" {
		return "", "", fmt.Errorf("desired generated tree is not available for comparison")
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", err
	}
	data, err := os.ReadFile(filepath.Join(resolved, "SKILL.md"))
	if err != nil {
		return "", "", err
	}
	name, err := skillName(data, filepath.Base(root))
	if err != nil {
		return "", "", err
	}
	fp, err := localprojection.FingerprintCopiedTree(resolved)
	if err != nil {
		return name, "unverified", nil
	}
	return name, fp, nil
}

func skillName(data []byte, fallback string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return "", fmt.Errorf("skill frontmatter is not verifiable")
	}
	end := 1
	for end < len(lines) && lines[end] != "---" {
		end++
	}
	if end == len(lines) {
		return "", fmt.Errorf("skill frontmatter is unterminated")
	}
	var header struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &header); err != nil {
		return "", err
	}
	if header.Name == "" {
		header.Name = fallback
	}
	return header.Name, nil
}

func targetKey(path string) string {
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Join(parent, filepath.Base(path))
}
