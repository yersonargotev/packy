package ci_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const accessContractScript = "#!/bin/sh\nprintf executed > \"$HOME/access-executed\"\n"
const accessContractLinks = "[Detail](references/detail.md) [Script](scripts/run.sh) [Notice](../../notices/mit) [Asset](assets/data.bin)\n"

// This authored catalog has duplicate native names and a same-/cross-Pack
// diamond. Expected bodies below are literal authored procedures, not a resolver.
func writeAccessCatalog(t *testing.T, revision, originCommit string) string {
	t.Helper()
	project := t.TempDir()
	for _, pack := range []string{"alpha", "beta", "zulu"} {
		version := "2.0.0"
		if revision == "next" {
			version = "2.1.0"
		}
		if pack == "alpha" {
			version = "1.0.0"
			if revision == "next" {
				version = "1.1.0"
			}
		}
		resources := []any{
			map[string]any{"kind": "asset", "id": "data", "source": "assets/data.bin", "description": "Supporting bytes", "requires": []string{}, "conflicts": []string{}, "bindings": []any{}, "surface_exclusions": []any{}},
			map[string]any{"kind": "notice", "id": "mit", "source": "notices/mit", "description": "Legal notice", "license": "MIT", "attribution": "Fixture Authors", "requires": []string{}, "conflicts": []string{}, "bindings": []any{}, "surface_exclusions": []any{}},
		}
		writeAccessFile(t, filepath.Join(project, "packs", pack, "assets/data.bin"), []byte{0, 255, 7, 10}, 0644)
		writeAccessFile(t, filepath.Join(project, "packs", pack, "notices/mit"), []byte("MIT License\nFixture Authors\n"), 0644)
		for _, id := range []string{"leaf", "left", "right", "root"} {
			requires := []string{"asset:data"}
			if id == "leaf" {
				requires = []string{}
			}
			dependencies := []any{}
			if id == "root" {
				requires = append(requires, "skill:left", "skill:right")
			}
			if id == "left" || id == "right" {
				requires = append(requires, "skill:leaf")
				if pack == "alpha" {
					dependencies = append(dependencies, map[string]any{"type": "skill", "target": "beta:leaf"})
				}
			}
			bindings := []any{}
			for _, host := range []string{"claude", "codex"} {
				invocation := "$" + id
				if host == "claude" {
					invocation = "/" + id
				}
				bindings = append(bindings, map[string]any{"surface": host, "projection": "skill", "name": id, "invocation": invocation, "mode": "native", "sharing": "exclusive", "capabilities": []any{}})
			}
			contract := map[string]any{"protocol": "skill-tree-v1", "entry_document": "ACCESS.md", "reference_files": []string{"ACCESS.md"}, "prerequisites": []string{"missing-content-runtime"}, "dependencies": dependencies}
			provenance := map[string]any{"id": "upstream", "path": "skill", "relationship": "adapted"}
			variants := []any{}
			for _, host := range []string{"claude", "codex"} {
				variants = append(variants, map[string]any{"surface": host, "source": "skills/" + id + "-" + host, "description": host + " " + id + " procedure", "origin": provenance})
			}
			resources = append(resources, map[string]any{"kind": "skill", "id": id, "source": "skills/" + id, "description": "Common " + id, "origin": provenance, "notices": []string{"notice:mit"}, "requires": requires, "conflicts": []string{}, "bindings": bindings, "surface_exclusions": []any{}, "variants": variants, "on_demand": contract})
			for _, host := range []string{"common", "claude", "codex"} {
				source := "skills/" + id
				if host != "common" {
					source += "-" + host
				}
				root := filepath.Join(project, "packs", pack, source)
				body := pack + " " + host + " " + id + " " + revision + "\n" + accessContractLinks
				if id == "root" {
					body += "[Left]({{packy.access:left}}) [Right]({{packy.access:right}}) [Again]({{packy.access:left}})\n"
				}
				if id == "left" || id == "right" {
					body += "[Leaf]({{packy.access:leaf}})\n"
					if pack == "alpha" {
						body += "[Foreign]({{packy.access:beta:leaf}})\n"
					}
				}
				writeAccessFile(t, filepath.Join(root, "SKILL.md"), []byte("---\nname: "+id+"\ndescription: Native fixture.\n---\nNative "+pack+" "+id+"\n"), 0644)
				writeAccessFile(t, filepath.Join(root, "ACCESS.md"), []byte(body), 0644)
				writeAccessFile(t, filepath.Join(root, "references/detail.md"), []byte("Complete "+pack+" "+host+" "+id+" "+revision+"\n"), 0644)
				writeAccessFile(t, filepath.Join(root, "assets/data.bin"), []byte{0, 255, 7, 10}, 0644)
				writeAccessFile(t, filepath.Join(root, "scripts/run.sh"), []byte(accessContractScript), 0755)
			}
		}
		requirements := []string{}
		if pack != "beta" {
			requirements = []string{"missing-pack-runtime"}
		}
		manifest := map[string]any{"schema_version": 3, "id": pack, "version": version, "description": "Access contract " + pack, "selectable": true, "surfaces": []string{"claude", "codex"}, "readiness_obligations": []string{"runtime-usability", "surface-authorization"}, "external_requirements": requirements, "origins": []any{map[string]any{"id": "upstream", "repository": "example/upstream", "commit": originCommit}}, "resources": resources}
		writeAccessJSON(t, filepath.Join(project, "packs", pack, "pack.json"), manifest)
	}
	return project
}

func writeAccessJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeAccessFile(t, path, data, 0644)
}

func writeAccessFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}

func buildAccessCatalog(t *testing.T, executable string, environment []string, project, identity string) (string, string) {
	t.Helper()
	assets := filepath.Join(t.TempDir(), "assets")
	out, diagnostic, err := runCatalogCLI(t, environment, t.TempDir(), executable, "catalog", "build", "--project", project, "--source-repository", "yersonargotev/packy-catalog", "--source-commit", identity, "--out-dir", assets, "--json")
	if err != nil {
		t.Fatalf("build access catalog: %v %s %s", err, out, diagnostic)
	}
	validateCatalogCLISchema(t, "catalog-build", out)
	return assets, strings.Fields(string(readIssue798File(t, filepath.Join(assets, "SHA256SUMS"))))[0]
}

func editAccessManifest(t *testing.T, project, pack string, edit func(map[string]any)) {
	t.Helper()
	path := filepath.Join(project, "packs", pack, "pack.json")
	var m map[string]any
	if err := json.Unmarshal(readIssue798File(t, path), &m); err != nil {
		t.Fatal(err)
	}
	edit(m)
	writeAccessJSON(t, path, m)
}

func accessCatalogSkill(m map[string]any, id string) map[string]any {
	for _, v := range m["resources"].([]any) {
		r := v.(map[string]any)
		if r["kind"] == "skill" && r["id"] == id {
			return r
		}
	}
	panic("missing authored fixture skill " + id)
}
