// Package skilltemplate renders explicitly reviewed references in a skill tree.
// It deliberately knows nothing about Pack manifests, host layouts, or lifecycle
// state so the same sealed bytes can be used by every surface adapter.
package skilltemplate

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

const (
	// RendererVersion is included in adapter provenance and rendered metadata so
	// a rendering behavior change cannot reuse bytes sealed by an older adapter.
	RendererVersion  = "skill-template/v1"
	directivePrefix  = "{{packy.skill:"
	directiveSuffix  = "}}"
	metadataFileName = ".packy-skill-template.json"
)

var logicalIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// File is one inert, regular output file in a rendered skill tree.
type File struct {
	Path    string
	Content []byte
	Mode    uint32
}

type renderMetadata struct {
	RendererVersion      string            `json:"renderer_version"`
	SourceContractSHA256 string            `json:"source_contract_sha256"`
	EffectiveNames       map[string]string `json:"effective_names"`
}

type sourceContractFile struct {
	path    string
	content []byte
	mode    uint32
}

// ReferencedSkills returns the canonical logical IDs named by explicit Packy
// directives. All other bytes are ignored.
func ReferencedSkills(data []byte) ([]string, error) {
	return referencedSkills(data, "packy.skill")
}

// ReferencedAccessSkills parses only explicit on-demand resource references.
func ReferencedAccessSkills(data []byte) ([]string, error) {
	return referencedSkills(data, "packy.access")
}

func referencedSkills(data []byte, namespace string) ([]string, error) {
	seen := map[string]bool{}
	for offset := 0; ; {
		relative := bytes.Index(data[offset:], []byte(namespace))
		if relative < 0 {
			break
		}
		namespaceStart := offset + relative
		openingStart := namespaceStart
		for openingStart > 0 && (data[openingStart-1] == ' ' || data[openingStart-1] == '\t' || data[openingStart-1] == '\r' || data[openingStart-1] == '\n') {
			openingStart--
		}
		braceStart := openingStart
		for braceStart > 0 && data[braceStart-1] == '{' {
			braceStart--
		}
		openingBraces := openingStart - braceStart
		if openingBraces < 2 {
			offset = namespaceStart + len(namespace)
			continue
		}
		if openingBraces != 2 || openingStart != namespaceStart {
			return nil, fmt.Errorf("malformed skill template directive at byte %d", braceStart)
		}
		start := braceStart
		valueStart := namespaceStart + len(namespace)
		if valueStart >= len(data) || data[valueStart] != ':' {
			return nil, fmt.Errorf("malformed skill template directive at byte %d", start)
		}
		valueStart++
		relativeEnd := bytes.Index(data[valueStart:], []byte(directiveSuffix))
		if relativeEnd < 0 {
			return nil, fmt.Errorf("unterminated skill template directive at byte %d", start)
		}
		end := valueStart + relativeEnd
		if end+len(directiveSuffix) < len(data) && data[end+len(directiveSuffix)] == '}' {
			return nil, fmt.Errorf("malformed skill template directive at byte %d", start)
		}
		logicalID := string(data[valueStart:end])
		if !logicalIDPattern.MatchString(logicalID) {
			return nil, fmt.Errorf("skill template directive at byte %d has invalid logical ID %q", start, logicalID)
		}
		seen[logicalID] = true
		offset = end + len(directiveSuffix)
	}
	result := make([]string, 0, len(seen))
	for logicalID := range seen {
		result = append(result, logicalID)
	}
	sort.Strings(result)
	return result, nil
}

// RenderTree copies a source tree into inert files, replaces only explicit
// reference directives, and sets SKILL.md's frontmatter name to the effective
// name selected for self. It never follows links or accepts special files.
func RenderTree(source, self string, names map[string]string) ([]File, error) {
	return renderTree(source, self, names, RendererVersion)
}

func renderTree(source, self string, names map[string]string, rendererVersion string) ([]File, error) {
	if !logicalIDPattern.MatchString(self) {
		return nil, fmt.Errorf("invalid template skill logical ID %q", self)
	}
	effectiveSelf, ok := names[self]
	if !ok || !logicalIDPattern.MatchString(effectiveSelf) {
		return nil, fmt.Errorf("template skill %q has no valid effective name", self)
	}
	rootInfo, err := os.Lstat(source)
	if err != nil {
		return nil, fmt.Errorf("inspect template skill source: %w", err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("template skill source %q must be a regular directory", source)
	}

	files := []File{}
	sourceFiles := []sourceContractFile{}
	effectiveNames := map[string]string{self: effectiveSelf}
	foundSkill := false
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == metadataFileName {
			return fmt.Errorf("template skill source contains reserved metadata path %q", metadataFileName)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			relative, _ := filepath.Rel(source, path)
			return fmt.Errorf("template skill source contains non-regular path %q", filepath.ToSlash(relative))
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sourceFiles = append(sourceFiles, sourceContractFile{path: relative, content: append([]byte(nil), content...), mode: uint32(info.Mode().Perm())})
		references, err := ReferencedSkills(content)
		if err != nil {
			return fmt.Errorf("render template file %q: %w", relative, err)
		}
		for _, logicalID := range references {
			name, ok := names[logicalID]
			if !ok || !logicalIDPattern.MatchString(name) {
				return fmt.Errorf("render template file %q: referenced skill %q has no valid effective name", relative, logicalID)
			}
			effectiveNames[logicalID] = name
		}
		content, err = renderDirectives(content, names)
		if err != nil {
			return fmt.Errorf("render template file %q: %w", relative, err)
		}
		if relative == "SKILL.md" {
			foundSkill = true
			content, err = renderFrontmatterName(content, effectiveSelf)
			if err != nil {
				return fmt.Errorf("render template file %q: %w", relative, err)
			}
		}
		files = append(files, File{Path: relative, Content: content, Mode: uint32(info.Mode().Perm())})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !foundSkill {
		return nil, fmt.Errorf("template skill source has no SKILL.md")
	}
	metadata, err := json.Marshal(renderMetadata{
		RendererVersion:      rendererVersion,
		SourceContractSHA256: sourceContractSHA256(sourceFiles),
		EffectiveNames:       effectiveNames,
	})
	if err != nil {
		return nil, fmt.Errorf("encode skill template metadata: %w", err)
	}
	metadata = append(metadata, '\n')
	files = append(files, File{Path: metadataFileName, Content: metadata, Mode: 0o644})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func sourceContractSHA256(files []sourceContractFile) string {
	sorted := append([]sourceContractFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].path < sorted[j].path })
	digest := sha256.New()
	digest.Write([]byte("packy-skill-template-source/v1\x00"))
	for _, file := range sorted {
		writeDigestBytes(digest, []byte(file.path))
		var mode [4]byte
		binary.BigEndian.PutUint32(mode[:], file.mode)
		digest.Write(mode[:])
		writeDigestBytes(digest, file.content)
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil))
}

func writeDigestBytes(digest hash.Hash, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	digest.Write(length[:])
	digest.Write(value)
}

func renderDirectives(data []byte, names map[string]string) ([]byte, error) {
	references, err := ReferencedSkills(data)
	if err != nil {
		return nil, err
	}
	if len(references) == 0 {
		return append([]byte(nil), data...), nil
	}
	replacements := make(map[string]string, len(references))
	for _, logicalID := range references {
		name, ok := names[logicalID]
		if !ok || !logicalIDPattern.MatchString(name) {
			return nil, fmt.Errorf("referenced skill %q has no valid effective name", logicalID)
		}
		replacements[directivePrefix+logicalID+directiveSuffix] = name
	}
	result := append([]byte(nil), data...)
	for _, logicalID := range references {
		token := directivePrefix + logicalID + directiveSuffix
		result = bytes.ReplaceAll(result, []byte(token), []byte(replacements[token]))
	}
	return result, nil
}

func renderFrontmatterName(data []byte, effectiveName string) ([]byte, error) {
	header, headerOffset, err := frontmatter(data)
	if err != nil {
		return nil, err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(header, &document); err != nil {
		return nil, fmt.Errorf("invalid YAML frontmatter: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("YAML frontmatter must be a mapping")
	}
	mapping := document.Content[0]
	var value *yaml.Node
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key, candidate := mapping.Content[i], mapping.Content[i+1]
		if key.Kind == yaml.ScalarNode && key.Value == "name" {
			if value != nil {
				return nil, fmt.Errorf("YAML frontmatter has duplicate name keys")
			}
			value = candidate
		}
	}
	if value == nil {
		return nil, fmt.Errorf("YAML frontmatter is missing a name scalar")
	}
	if value.Kind != yaml.ScalarNode || value.Tag != "!!str" && value.Tag != "" {
		return nil, fmt.Errorf("YAML frontmatter name must be a string scalar")
	}
	if value.Style != 0 && value.Style != yaml.SingleQuotedStyle && value.Style != yaml.DoubleQuotedStyle {
		return nil, fmt.Errorf("YAML frontmatter name uses an unsupported multiline scalar")
	}
	start, end, quote, err := yamlScalarRange(header, value)
	if err != nil {
		return nil, err
	}
	replacement := []byte(effectiveName)
	if quote != 0 {
		replacement = append([]byte{quote}, replacement...)
		replacement = append(replacement, quote)
	}
	absoluteStart, absoluteEnd := headerOffset+start, headerOffset+end
	result := make([]byte, 0, len(data)-absoluteEnd+absoluteStart+len(replacement))
	result = append(result, data[:absoluteStart]...)
	result = append(result, replacement...)
	result = append(result, data[absoluteEnd:]...)
	return result, nil
}

// frontmatter returns only the YAML bytes, plus their byte offset in data.
func frontmatter(data []byte) ([]byte, int, error) {
	firstEnd := bytes.IndexByte(data, '\n')
	if firstEnd < 0 || string(bytes.TrimSuffix(data[:firstEnd], []byte{'\r'})) != "---" {
		return nil, 0, fmt.Errorf("SKILL.md has no YAML frontmatter")
	}
	headerOffset := firstEnd + 1
	for offset := headerOffset; offset <= len(data); {
		relativeEnd := bytes.IndexByte(data[offset:], '\n')
		lineEnd := len(data)
		next := len(data) + 1
		if relativeEnd >= 0 {
			lineEnd = offset + relativeEnd
			next = lineEnd + 1
		}
		line := bytes.TrimSuffix(data[offset:lineEnd], []byte{'\r'})
		if string(line) == "---" {
			return data[headerOffset:offset], headerOffset, nil
		}
		if next > len(data) {
			break
		}
		offset = next
	}
	return nil, 0, fmt.Errorf("SKILL.md YAML frontmatter is unterminated")
}

func yamlScalarRange(header []byte, value *yaml.Node) (int, int, byte, error) {
	if value.Line < 1 || value.Column < 1 {
		return 0, 0, 0, fmt.Errorf("YAML frontmatter name has no source location")
	}
	lineStart := 0
	for line := 1; line < value.Line; line++ {
		relative := bytes.IndexByte(header[lineStart:], '\n')
		if relative < 0 {
			return 0, 0, 0, fmt.Errorf("YAML frontmatter name source location is invalid")
		}
		lineStart += relative + 1
	}
	lineEnd := len(header)
	if relative := bytes.IndexByte(header[lineStart:], '\n'); relative >= 0 {
		lineEnd = lineStart + relative
	}
	line := header[lineStart:lineEnd]
	columnOffset, ok := byteOffsetForColumn(line, value.Column)
	if !ok {
		return 0, 0, 0, fmt.Errorf("YAML frontmatter name column is invalid")
	}
	start := lineStart + columnOffset
	if start >= lineEnd {
		return 0, 0, 0, fmt.Errorf("YAML frontmatter name scalar is empty")
	}
	quote := byte(0)
	end := start
	switch header[start] {
	case '\'':
		quote = '\''
		end++
		for end < lineEnd {
			if header[end] != '\'' {
				end++
				continue
			}
			if end+1 < lineEnd && header[end+1] == '\'' {
				end += 2
				continue
			}
			end++
			break
		}
	case '"':
		quote = '"'
		end++
		escaped := false
		for end < lineEnd {
			character := header[end]
			end++
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
				continue
			}
			if character == '"' {
				break
			}
		}
	default:
		for end < lineEnd && !strings.ContainsRune(" \t\r,[]{}#", rune(header[end])) {
			end++
		}
	}
	if end <= start || quote != 0 && (end > lineEnd || header[end-1] != quote) {
		return 0, 0, 0, fmt.Errorf("YAML frontmatter name scalar cannot be patched safely")
	}
	var decoded string
	if err := yaml.Unmarshal(header[start:end], &decoded); err != nil || decoded != value.Value {
		return 0, 0, 0, fmt.Errorf("YAML frontmatter name scalar cannot be patched safely")
	}
	return start, end, quote, nil
}

func byteOffsetForColumn(line []byte, column int) (int, bool) {
	if column < 1 {
		return 0, false
	}
	offset := 0
	for current := 1; current < column; current++ {
		if offset >= len(line) {
			return 0, false
		}
		_, size := utf8.DecodeRune(line[offset:])
		if size == 0 {
			return 0, false
		}
		offset += size
	}
	return offset, true
}
