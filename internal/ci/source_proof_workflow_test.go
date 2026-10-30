package ci_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func releaseSourceProofGuard(content string) error {
	type step struct {
		Name     string            `yaml:"name"`
		Run      string            `yaml:"run"`
		If       any               `yaml:"if"`
		Continue any               `yaml:"continue-on-error"`
		Env      map[string]string `yaml:"env"`
		Shell    any               `yaml:"shell"`
		Uses     string            `yaml:"uses"`
		With     map[string]string `yaml:"with"`
	}
	var w struct {
		Defaults any `yaml:"defaults"`
		Jobs     map[string]struct {
			If       any       `yaml:"if"`
			Needs    yaml.Node `yaml:"needs"`
			Defaults any       `yaml:"defaults"`
			Continue any       `yaml:"continue-on-error"`
			Steps    []step    `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(content), &w); err != nil {
		return err
	}
	if strings.Contains(content, "validate-packy.sh") {
		return fmt.Errorf("release repeats source validator")
	}
	prepare, ok := w.Jobs["prepare"]
	if !ok || prepare.If != nil || prepare.Continue != nil || prepare.Defaults != nil || w.Defaults != nil {
		return fmt.Errorf("conditional prepare")
	}
	var needs []string
	publish := w.Jobs["publish"]
	if err := publish.Needs.Decode(&needs); err != nil || !slices.Contains(needs, "prepare") || !slices.Contains(needs, "contracts") {
		return fmt.Errorf("publication dependencies missing")
	}
	proof, build, retain, upload := -1, -1, -1, -1
	for i, s := range prepare.Steps {
		if strings.Contains(s.Run, "internal/tools/ciproof") {
			if proof >= 0 || s.If != nil || s.Continue != nil || s.Shell != nil || s.Env["GH_TOKEN"] != "${{ github.token }}" || strings.TrimSpace(s.Run) != `go run ./internal/tools/ciproof --sha "$GITHUB_SHA" > "$RUNNER_TEMP/source-ci-proof.json"` {
				return fmt.Errorf("invalid proof gate")
			}
			proof = i
		}
		if strings.Contains(s.Run, "build-release-artifacts.sh") {
			if s.If != nil || s.Continue != nil || !strings.Contains(s.Run, `--version "$RELEASE_TAG" --out-dir dist`) || s.Env["RELEASE_TAG"] != "${{ steps.release.outputs.tag }}" {
				return fmt.Errorf("invalid build")
			}
			build = i
		}
		if strings.Contains(s.Run, `cp "$RUNNER_TEMP/source-ci-proof.json" release-metadata/source-ci-proof.json`) {
			if s.If != nil || s.Continue != nil || s.Shell != nil || !strings.Contains(s.Run, "$GITHUB_STEP_SUMMARY") || !slices.Contains(strings.Split(s.Run, "\n"), `cp "$RUNNER_TEMP/source-ci-proof.json" release-metadata/source-ci-proof.json`) || !strings.Contains(s.Run, "set -euo pipefail") {
				return fmt.Errorf("invalid retained proof")
			}
			retain = i
		}
		if strings.HasPrefix(s.Uses, "actions/upload-artifact@") && s.Name == "Retain validated release bundle" {
			if s.If != nil || s.Continue != nil || s.With["name"] != "packy-release-${{ steps.release.outputs.tag }}" || !slices.Contains(strings.Fields(s.With["path"]), "release-metadata") || !slices.Contains(strings.Fields(s.With["path"]), "dist") || s.With["if-no-files-found"] != "error" {
				return fmt.Errorf("invalid retained bundle")
			}
			upload = i
		}
	}
	if proof < 0 || build <= proof || retain <= build || upload <= retain {
		return fmt.Errorf("proof must precede build and retention")
	}
	return nil
}

func TestReleaseSourceProofGate(t *testing.T) {
	text := readFile(t, filepath.Join(repositoryRoot(t), ".github", "workflows", "release.yml"))
	if err := releaseSourceProofGuard(text); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(string) string{
		"duplicate validator": func(s string) string {
			return strings.Replace(s, "      - name: Verify exact-commit main CI proof\n", "      - name: Repeat validator\n        run: ./scripts/validate-packy.sh --ci\n      - name: Verify exact-commit main CI proof\n", 1)
		},
		"removed gate": func(s string) string {
			return strings.Replace(s, "./internal/tools/ciproof", "./internal/tools/other", 1)
		},
		"conditional gate": func(s string) string {
			return strings.Replace(s, "      - name: Verify exact-commit main CI proof\n", "      - name: Verify exact-commit main CI proof\n        if: false\n", 1)
		},
		"masked failure": func(s string) string {
			return strings.Replace(s, "      - name: Verify exact-commit main CI proof\n", "      - name: Verify exact-commit main CI proof\n        continue-on-error: true\n", 1)
		},
		"masked command": func(s string) string {
			return strings.Replace(s, `> "$RUNNER_TEMP/source-ci-proof.json"`, `> "$RUNNER_TEMP/source-ci-proof.json" || true`, 1)
		},
		"custom shell": func(s string) string {
			return strings.Replace(s, "      - name: Verify exact-commit main CI proof\n", "      - name: Verify exact-commit main CI proof\n        shell: bash {0}\n", 1)
		},
		"job defaults": func(s string) string {
			return strings.Replace(s, "    name: Build and validate release\n", "    name: Build and validate release\n    defaults:\n      run:\n        shell: bash {0}\n", 1)
		},
		"no artifact": func(s string) string {
			return strings.Replace(s, "            release-metadata\n", "            other\n", 1)
		},
		"no dependency": func(s string) string {
			return strings.Replace(s, "    needs: [prepare, contracts]", "    needs: [contracts]", 1)
		},
		"proof removed": func(s string) string {
			return strings.Replace(s, "cp \"$RUNNER_TEMP/source-ci-proof.json\"", "cp \"$RUNNER_TEMP/other.json\"", 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := releaseSourceProofGuard(mutate(text)); err == nil {
				t.Fatal("accepted unsafe workflow")
			}
		})
	}
}
