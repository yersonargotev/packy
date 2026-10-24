package ci_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/yersonargotev/packy/internal/testprocess"
)

func TestCatalogExecutable(t *testing.T) {
	executable, sourceCommit := buildCatalogCLI(t)
	testCatalogExecutable(t, executable, "v0.0.0", sourceCommit)
}

// Release jobs supply the extracted artifact; this entry point never rebuilds it.
func TestCatalogReleaseExecutable(t *testing.T) {
	executable := os.Getenv("PACKY_RELEASE_BINARY")
	if executable == "" {
		t.Skip("staged release binary not supplied")
	}
	version := os.Getenv("PACKY_RELEASE_VERSION")
	if version == "" || !filepath.IsAbs(executable) {
		t.Fatal("release contracts require an absolute binary path and expected version")
	}
	commit, stderr, err := runCatalogCLI(t, testprocess.Env(t), repositoryRoot(t), "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("release source commit: %v %s", err, stderr)
	}
	testCatalogExecutable(t, executable, version, strings.TrimSpace(string(commit)))
}

func testCatalogExecutable(t *testing.T, executable, version, sourceCommit string) {
	t.Run("identity", func(t *testing.T) {
		stdout, stderr, err := runCatalogCLI(t, testprocess.Env(t), t.TempDir(), executable, "version", "--json")
		if err != nil {
			t.Fatalf("identity: %v\n%s\n%s", err, stdout, stderr)
		}
		validateCatalogCLISchema(t, "packy-version", stdout)
		var identity struct {
			Version      string `json:"version"`
			SourceCommit string `json:"source_commit"`
		}
		if err := json.Unmarshal(stdout, &identity); err != nil {
			t.Fatal(err)
		}
		if identity.Version != version || identity.SourceCommit != sourceCommit {
			t.Fatalf("identity = %s; want version %s source %s", stdout, version, sourceCommit)
		}
	})
	t.Run("validate", func(t *testing.T) { testCatalogValidationExecutable(t, executable) })
	t.Run("build", func(t *testing.T) { testCatalogBuildExecutable(t, executable, sourceCommit) })
	t.Run("origin reuse", func(t *testing.T) { testCatalogOriginReuse(t, executable) })
	t.Run("publication", func(t *testing.T) { testCatalogPublicationExecutable(t, executable) })
	t.Run("candidate", func(t *testing.T) { testCatalogCandidateExecutable(t, executable) })
	t.Run("candidate transitions", func(t *testing.T) { testCatalogCandidateTransitionsExecutable(t, executable) })
	t.Run("on-demand access", func(t *testing.T) { testCatalogOnDemandExecutable(t, executable) })
}

func testCatalogValidationExecutable(t *testing.T, executable string) {
	t.Run("argument output contract", func(t *testing.T) {
		testCatalogArgumentOutput(t, executable, "validate")
	})
	project := t.TempDir()
	writePublicationCatalogPack(t, project, "alpha", "1.0.0", "inert skill\n")
	environment := testprocess.Env(t)
	directory := t.TempDir()
	t.Run("complete catalog", func(t *testing.T) {
		stdout, stderr, err := runCatalogCLI(t, environment, directory, executable, "catalog", "validate", "--project", project, "--json")
		if err != nil {
			t.Fatalf("validate: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		validateCatalogCLISchema(t, "catalog-validation", stdout)
		var result struct {
			SchemaVersion int    `json:"schema_version"`
			Report        string `json:"report"`
			Result        string `json:"result"`
			Packs         []struct {
				ID      string `json:"id"`
				Version string `json:"version"`
			} `json:"packs"`
			FitnessRows int `json:"fitness_rows"`
		}
		if err := json.Unmarshal(stdout, &result); err != nil {
			t.Fatal(err)
		}
		if result.SchemaVersion != 1 || result.Report != "catalog-validation" || result.Result != "valid" || len(result.Packs) != 1 || result.Packs[0].ID != "alpha" || result.Packs[0].Version != "1.0.0" || result.FitnessRows == 0 {
			t.Fatalf("unexpected validation: %s", stdout)
		}
	})
	t.Run("baseline and failures", func(t *testing.T) {
		baseline := t.TempDir()
		writePublicationCatalogPack(t, baseline, "alpha", "1.0.0", "original\n")
		for _, tc := range []struct {
			name, version, content string
			valid                  bool
		}{
			{"unchanged", "1.0.0", "original\n", true},
			{"changed and bumped", "1.1.0", "changed\n", true},
			{"changed without bump", "1.0.0", "changed\n", false},
			{"downgrade", "0.9.0", "changed\n", false},
			{"unchanged with bump", "1.1.0", "original\n", false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				candidate := t.TempDir()
				writePublicationCatalogPack(t, candidate, "alpha", tc.version, tc.content)
				stdout, stderr, err := runCatalogCLI(t, environment, directory, executable, "catalog", "validate", "--project", candidate, "--baseline", baseline, "--json")
				assertCatalogResult(t, stdout, stderr, err, tc.valid, "validation_failed")
			})
		}
	})
	t.Run("whole catalog rejection", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			mutate func(string)
		}{
			{"invalid unselected pack", func(candidate string) { writePublicationCatalogPack(t, candidate, "zulu", "not-semver", "invalid\n") }},
			{"undeclared file", func(candidate string) {
				if err := os.WriteFile(filepath.Join(candidate, "packs", "alpha", "orphan"), []byte("orphan"), 0o600); err != nil {
					t.Fatal(err)
				}
			}},
			{"malformed manifest", func(candidate string) {
				if err := os.WriteFile(filepath.Join(candidate, "packs", "alpha", "pack.json"), []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
			}},
			{"unknown provenance", func(candidate string) {
				mutateCatalogManifest(t, candidate, func(m map[string]any) {
					m["resources"].([]any)[0].(map[string]any)["origin"] = map[string]any{"id": "missing", "path": "skill", "relationship": "exact-copy"}
				})
			}},
			{"runtime fitness", func(candidate string) {
				mutateCatalogManifest(t, candidate, func(m map[string]any) {
					resources := m["resources"].([]any)
					raw, err := json.Marshal(resources[0])
					if err != nil {
						t.Fatal(err)
					}
					var duplicate map[string]any
					if err := json.Unmarshal(raw, &duplicate); err != nil {
						t.Fatal(err)
					}
					duplicate["id"] = "beta"
					duplicate["source"] = "skills/beta"
					m["resources"] = append(resources, duplicate)
					path := filepath.Join(candidate, "packs", "alpha", "skills", "beta")
					if err := os.MkdirAll(path, 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("collision\n"), 0o644); err != nil {
						t.Fatal(err)
					}
				})
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				candidate := t.TempDir()
				writePublicationCatalogPack(t, candidate, "alpha", "1.0.0", "valid\n")
				tc.mutate(candidate)
				stdout, stderr, err := runCatalogCLI(t, environment, directory, executable, "catalog", "validate", "--project", candidate, "--json")
				assertCatalogResult(t, stdout, stderr, err, false, "validation_failed")
			})
		}
	})
	t.Run("inert and no personal state", func(t *testing.T) {
		candidate := t.TempDir()
		marker := filepath.Join(candidate, "executed")
		writePublicationCatalogPack(t, candidate, "alpha", "1.0.0", "#!/bin/sh\ntouch "+marker+"\n")
		if err := os.Chmod(filepath.Join(candidate, "packs", "alpha", "skills", "alpha", "SKILL.md"), 0o755); err != nil {
			t.Fatal(err)
		}
		roots := []string{candidate, directory}
		for _, entry := range environment {
			if key, value, ok := strings.Cut(entry, "="); ok && (key == "HOME" || key == "XDG_CONFIG_HOME" || key == "TMPDIR") {
				roots = append(roots, value)
			}
		}
		before := catalogTreeState(t, roots)
		stdout, stderr, err := runCatalogCLI(t, environment, directory, executable, "catalog", "validate", "--project", candidate)
		if err != nil || len(stdout) == 0 {
			t.Fatalf("human validation: %v %s %s", err, stdout, stderr)
		}
		if after := catalogTreeState(t, roots); !reflect.DeepEqual(before, after) {
			t.Fatalf("validation changed files: before=%v after=%v", before, after)
		}
	})
	t.Run("invalid baseline and arguments", func(t *testing.T) {
		for _, args := range [][]string{
			{"--project", project, "--baseline", t.TempDir()},
			{"--project", t.TempDir()},
			{"unexpected"},
		} {
			stdout, stderr, err := runCatalogCLI(t, environment, directory, executable, append([]string{"catalog", "validate", "--json"}, args...)...)
			code := "validation_failed"
			if args[0] == "unexpected" {
				code = "invalid_arguments"
			}
			assertCatalogResult(t, stdout, stderr, err, false, code)
		}
	})
	t.Run("exact origin bytes", func(t *testing.T) {
		commit, proxy, caFile := catalogGitOrigin(t)
		candidate := t.TempDir()
		writeCatalogOriginPack(t, candidate, commit)
		untrusted := testprocess.Env(t, "HTTPS_PROXY="+proxy)
		stdout, stderr, err := runCatalogCLI(t, untrusted, directory, executable, "catalog", "validate", "--project", candidate, "--json")
		assertCatalogResult(t, stdout, stderr, err, false, "validation_failed")
		if !bytes.Contains(stdout, []byte("certificate")) {
			t.Fatalf("untrusted origin did not fail TLS verification: %s", stdout)
		}
		isolated := testprocess.Env(t, "HTTPS_PROXY="+proxy, "SSL_CERT_FILE="+caFile)
		stdout, stderr, err = runCatalogCLI(t, isolated, directory, executable, "catalog", "validate", "--project", candidate, "--json")
		assertCatalogResult(t, stdout, stderr, err, true, "")
		if err := os.WriteFile(filepath.Join(candidate, "packs", "alpha", "skills", "alpha", "SKILL.md"), []byte("divergent local bytes\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		stdout, stderr, err = runCatalogCLI(t, isolated, directory, executable, "catalog", "validate", "--project", candidate, "--json")
		assertCatalogResult(t, stdout, stderr, err, false, "validation_failed")
		if !bytes.Contains(stdout, []byte("exact-copy")) {
			t.Fatalf("expected origin-byte comparison failure: %s", stdout)
		}
	})
	t.Run("controlled origin acquisition failure", func(t *testing.T) {
		var requests atomic.Int32
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			http.Error(w, "controlled origin unavailable", http.StatusBadGateway)
		}))
		defer proxy.Close()
		candidate := t.TempDir()
		writeCatalogOriginPack(t, candidate, strings.Repeat("a", 40))
		isolated := testprocess.Env(t, "HTTPS_PROXY="+proxy.URL, "HTTP_PROXY="+proxy.URL)
		stdout, stderr, err := runCatalogCLI(t, isolated, directory, executable, "catalog", "validate", "--project", candidate, "--json")
		assertCatalogResult(t, stdout, stderr, err, false, "validation_failed")
		if requests.Load() == 0 {
			t.Fatalf("origin acquisition never reached controlled transport: %s", stdout)
		}
	})
	t.Run("output failure", func(t *testing.T) {
		output, err := os.Open(filepath.Join(project, "packs", "alpha", "pack.json"))
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "catalog", "validate", "--project", project, "--json")
		command.Env = environment
		command.Dir = directory
		command.Stdout = output
		if err := command.Run(); err == nil {
			t.Fatal("writing to a read-only output succeeded")
		}
	})

}

func runCatalogCLI(t *testing.T, environment []string, directory, executable string, args ...string) ([]byte, []byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, args...)
	command.Env = environment
	command.Dir = directory
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func assertCatalogResult(t *testing.T, stdout, stderr []byte, runErr error, valid bool, code string) {
	t.Helper()
	validateCatalogCLISchema(t, "catalog-validation", stdout)
	var result struct {
		SchemaVersion int    `json:"schema_version"`
		Report        string `json:"report"`
		Result        string `json:"result"`
		Error         *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("JSON: %v; run=%v stdout=%s stderr=%s", err, runErr, stdout, stderr)
	}
	if result.SchemaVersion != 1 || result.Report != "catalog-validation" {
		t.Fatalf("report = %s", stdout)
	}
	if valid {
		if runErr != nil || result.Result != "valid" || result.Error != nil {
			t.Fatalf("expected valid: %v %s %s", runErr, stdout, stderr)
		}
		return
	}
	if runErr == nil || result.Result != "invalid" || result.Error == nil || result.Error.Code != code || result.Error.Message == "" {
		t.Fatalf("expected invalid: %v %s %s", runErr, stdout, stderr)
	}
}

func mutateCatalogManifest(t *testing.T, root string, mutate func(map[string]any)) {
	t.Helper()
	path := filepath.Join(root, "packs", "alpha", "pack.json")
	var manifest map[string]any
	if err := json.Unmarshal(readIssue798File(t, path), &manifest); err != nil {
		t.Fatal(err)
	}
	mutate(manifest)
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func catalogTreeState(t *testing.T, roots []string) map[string]string {
	t.Helper()
	state := map[string]string{}
	for _, root := range roots {
		if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			state[path] = info.Mode().String()
			if entry.Type()&os.ModeSymlink != 0 {
				target, err := os.Readlink(path)
				if err != nil {
					return err
				}
				state[path] += target
			} else if !entry.IsDir() {
				state[path] += string(readIssue798File(t, path))
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	return state
}

func validateCatalogCLISchema(t *testing.T, name string, data []byte) {
	t.Helper()
	path := filepath.Join(repositoryRoot(t), "schemas", "cli", "v1", name+".schema.json")
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile(path)
	if err != nil {
		t.Fatal(err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, data)
	}
}

func writeCatalogOriginPack(t *testing.T, candidate, commit string) {
	t.Helper()
	writePublicationCatalogPack(t, candidate, "alpha", "1.0.0", "upstream skill\n")
	notice := filepath.Join(candidate, "packs", "alpha", "notices", "mit")
	if err := os.MkdirAll(filepath.Dir(notice), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notice, []byte("MIT License\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mutateCatalogManifest(t, candidate, func(m map[string]any) {
		m["origins"] = []any{map[string]any{"id": "upstream", "repository": "example/upstream", "commit": commit}}
		resources := m["resources"].([]any)
		resource := resources[0].(map[string]any)
		resource["origin"] = map[string]any{"id": "upstream", "path": "skill", "relationship": "exact-copy"}
		resource["notices"] = []string{"notice:mit"}
		m["resources"] = []any{map[string]any{"kind": "notice", "id": "mit", "source": "notices/mit", "description": "MIT notice", "license": "MIT", "attribution": "Example", "requires": []string{}, "conflicts": []string{}, "bindings": []any{}, "surface_exclusions": []any{}}, resource}
	})
}

func buildCatalogCLI(t *testing.T) (string, string) {
	t.Helper()
	root := repositoryRoot(t)
	executable := filepath.Join(t.TempDir(), "packy")
	buildEnvironment := testprocess.GoOfflineEnv(t)
	commitOutput, stderr, err := runCatalogCLI(t, testprocess.Env(t), root, "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("source commit: %v %s", err, stderr)
	}
	sourceCommit := strings.TrimSpace(string(commitOutput))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-ldflags=-X github.com/yersonargotev/packy/internal/version.Value=v0.0.0 -X github.com/yersonargotev/packy/internal/version.SourceCommit="+sourceCommit, "-o", executable, "./cmd/packy")
	build.Env = buildEnvironment
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build executable: %v\n%s", err, output)
	}
	return executable, sourceCommit
}

func testCatalogArgumentOutput(t *testing.T, executable, command string) {
	t.Helper()
	environment := testprocess.Env(t)
	directory := t.TempDir()
	for _, args := range [][]string{
		{"--builder", "example/other", "--json"},
		{"--unknown", "value", "--json"},
		{"--json", "--unknown", "value"},
		{"--help=invalid", "--json"},
		{"--json", "--help=invalid"},
		{"---invalid", "--json"},
		{"--json", "---invalid"},
		{"--=invalid", "--json"},
		{"--json", "--=invalid"},
		{"--json=invalid", "--json"},
		{"--json", "--json=invalid"},
		{"--json", "--project"},
		{"unexpected", "--json"},
		{"--json=false", "--unknown", "--json=true"},
	} {
		t.Run("JSON "+strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, err := runCatalogCLI(t, environment, directory, executable, append([]string{"catalog", command}, args...)...)
			if command == "build" {
				assertCatalogBuildFailure(t, stdout, stderr, err, "invalid_arguments")
			} else {
				assertCatalogResult(t, stdout, stderr, err, false, "invalid_arguments")
			}
		})
	}
	for _, args := range [][]string{
		{"--project"},
		{"--project", "--json"},
		{"--", "--json"},
		{"---invalid", "--json=false"},
		{"--json", "--unknown", "--json=false"},
	} {
		t.Run("human "+strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, err := runCatalogCLI(t, environment, directory, executable, append([]string{"catalog", command}, args...)...)
			if err == nil || len(stdout) != 0 || len(stderr) == 0 {
				t.Fatalf("human argument failure: %v stdout=%s stderr=%s", err, stdout, stderr)
			}
		})
	}
	t.Run("help", func(t *testing.T) {
		stdout, stderr, err := runCatalogCLI(t, environment, directory, executable, "catalog", command, "--help")
		if err != nil || len(stdout) == 0 || len(stderr) != 0 {
			t.Fatalf("help: %v stdout=%s stderr=%s", err, stdout, stderr)
		}
	})
}
