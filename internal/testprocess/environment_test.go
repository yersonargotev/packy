package testprocess

import (
	"archive/zip"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestEnvReturnsTheExactIsolatedChildContract(t *testing.T) {
	t.Setenv("PATH", "/test/bin")
	t.Setenv("PACKY_AMBIENT_SECRET", "must-not-cross")

	environment := Env(t)
	values := environmentMap(t, environment)
	root := filepath.Dir(values["HOME"])
	want := map[string]string{
		"GIT_CONFIG_COUNT":    "2",
		"GIT_CONFIG_GLOBAL":   os.DevNull,
		"GIT_CONFIG_KEY_0":    "maintenance.auto",
		"GIT_CONFIG_KEY_1":    "gc.auto",
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_CONFIG_VALUE_0":  "false",
		"GIT_CONFIG_VALUE_1":  "0",
		"GIT_TERMINAL_PROMPT": "0",
		"GO_TELEMETRY_CHILD":  "2",
		"HOME":                filepath.Join(root, "home"),
		"LANG":                "C",
		"LC_ALL":              "C",
		"PATH":                "/test/bin",
		"TMPDIR":              filepath.Join(root, "tmp"),
		"XDG_CACHE_HOME":      filepath.Join(root, "xdg", "cache"),
		"XDG_CONFIG_HOME":     filepath.Join(root, "xdg", "config"),
		"XDG_DATA_HOME":       filepath.Join(root, "xdg", "data"),
		"XDG_STATE_HOME":      filepath.Join(root, "xdg", "state"),
	}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("Env() = %#v, want %#v", values, want)
	}
	if strings.Contains(strings.Join(environment, "\n"), "PACKY_AMBIENT_SECRET") {
		t.Fatalf("Env() inherited an ambient sentinel: %q", environment)
	}
	if !sort.StringsAreSorted(environment) {
		t.Fatalf("Env() is not sorted: %q", environment)
	}
	for _, key := range []string{"HOME", "TMPDIR", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		info, err := os.Stat(values[key])
		if err != nil {
			t.Fatalf("stat %s: %v", key, err)
		}
		if !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode = %v, want owner-only directory", key, info.Mode())
		}
	}
}

func TestEnvMergesExplicitAdditionsAndPermitsPathReplacement(t *testing.T) {
	t.Setenv("PATH", "/host/bin")

	environment := Env(t, "FIXTURE_MARKER=value=with=equals", "PATH=/fixture/bin")
	values := environmentMap(t, environment)
	if got := values["FIXTURE_MARKER"]; got != "value=with=equals" {
		t.Fatalf("FIXTURE_MARKER = %q, want caller value", got)
	}
	if got := values["PATH"]; got != "/fixture/bin" {
		t.Fatalf("PATH = %q, want permitted replacement", got)
	}
	if !sort.StringsAreSorted(environment) {
		t.Fatalf("Env() with additions is not sorted: %q", environment)
	}
}

func TestEnvIsObservedByARealReexecChild(t *testing.T) {
	t.Setenv("PACKY_AMBIENT_SECRET", "must-not-cross")
	command := exec.Command(os.Args[0], "-test.run=^TestEnvReexecChildHelper$")
	command.Env = Env(t, "PACKY_TESTPROCESS_CHILD=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("re-exec isolated child: %v\n%s", err, output)
	}
}

func TestEnvReexecChildHelper(t *testing.T) {
	if os.Getenv("PACKY_TESTPROCESS_CHILD") == "" {
		return
	}
	for key, want := range map[string]string{
		"LANG":               "C",
		"LC_ALL":             "C",
		"GO_TELEMETRY_CHILD": "2",
	} {
		if got := os.Getenv(key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
	if got := os.Getenv("PACKY_AMBIENT_SECRET"); got != "" {
		t.Fatalf("re-exec child inherited ambient sentinel %q", got)
	}
	for _, key := range []string{"HOME", "TMPDIR", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		if info, err := os.Stat(os.Getenv(key)); err != nil || !info.IsDir() {
			t.Fatalf("%s does not name an existing directory: %v", key, err)
		}
	}
}

func TestEnvRejectsAnAbsentPATHBeforeConstructingAChildEnvironment(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestEnvAbsentPATHHelper$")
	// This literal minimal environment is the contract under test: PATH must be absent.
	command.Env = []string{"PACKY_TESTPROCESS_ABSENT_PATH=1"}
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("Env() accepted an absent PATH:\n%s", output)
	}
	if !strings.Contains(string(output), "PATH is empty") {
		t.Fatalf("Env() returned the wrong error: %v\n%s", err, output)
	}
}

func TestEnvAbsentPATHHelper(t *testing.T) {
	if os.Getenv("PACKY_TESTPROCESS_ABSENT_PATH") == "" {
		return
	}
	Env(t)
}

func TestEnvMakesRealGitIgnoreHostConfigurationAndAutomaticMaintenance(t *testing.T) {
	gitExecutable, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	hostileGlobal := filepath.Join(t.TempDir(), "global.gitconfig")
	hostileSystem := filepath.Join(t.TempDir(), "system.gitconfig")
	for _, path := range []string{hostileGlobal, hostileSystem} {
		contents := "[packy]\n\thost-poison = visible\n[maintenance]\n\tauto = true\n[gc]\n\tauto = 999\n"
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GIT_CONFIG_GLOBAL", hostileGlobal)
	t.Setenv("GIT_CONFIG_SYSTEM", hostileSystem)
	environment := Env(t)

	for key, want := range map[string]string{"maintenance.auto": "false", "gc.auto": "0"} {
		command := exec.Command(gitExecutable, "config", "--get", key)
		command.Env = environment
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git config --get %s: %v\n%s", key, err, output)
		}
		if got := strings.TrimSpace(string(output)); got != want {
			t.Fatalf("git %s = %q, want %q", key, got, want)
		}
	}
	command := exec.Command(gitExecutable, "config", "--get", "packy.host-poison")
	command.Env = environment
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatalf("git observed hostile host configuration: %s", output)
	}
}

func TestEnvRejectsMalformedDuplicateAndProtectedAdditions(t *testing.T) {
	t.Setenv("PATH", "/host/bin")
	tests := map[string][]string{
		"missing equals":           {"MALFORMED"},
		"empty key":                {"=value"},
		"NUL in key":               {"BAD\x00KEY=value"},
		"NUL in value":             {"BAD=value\x00tail"},
		"duplicate caller key":     {"DUPLICATE=first", "DUPLICATE=second"},
		"HOME":                     {"HOME=/replacement"},
		"TMPDIR":                   {"TMPDIR=/replacement"},
		"LANG":                     {"LANG=en_US.UTF-8"},
		"LC_ALL":                   {"LC_ALL=en_US.UTF-8"},
		"XDG_CACHE_HOME":           {"XDG_CACHE_HOME=/replacement"},
		"XDG_CONFIG_HOME":          {"XDG_CONFIG_HOME=/replacement"},
		"XDG_DATA_HOME":            {"XDG_DATA_HOME=/replacement"},
		"XDG_STATE_HOME":           {"XDG_STATE_HOME=/replacement"},
		"GO_TELEMETRY_CHILD":       {"GO_TELEMETRY_CHILD=0"},
		"GIT_CONFIG_GLOBAL":        {"GIT_CONFIG_GLOBAL=/replacement"},
		"GIT_CONFIG_NOSYSTEM":      {"GIT_CONFIG_NOSYSTEM=0"},
		"GIT_TERMINAL_PROMPT":      {"GIT_TERMINAL_PROMPT=1"},
		"GIT_CONFIG_COUNT":         {"GIT_CONFIG_COUNT=0"},
		"GIT_CONFIG_KEY pattern":   {"GIT_CONFIG_KEY_9=core.editor"},
		"GIT_CONFIG_VALUE pattern": {"GIT_CONFIG_VALUE_9=unsafe"},
	}
	for name, additions := range tests {
		t.Run(name, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestEnvRejectionHelper$")
			command.Env = Env(t, "PACKY_TESTPROCESS_REJECTION="+name)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatalf("Env() accepted additions %q:\n%s", additions, output)
			}
			if !strings.Contains(string(output), "construct test process environment") {
				t.Fatalf("Env() rejection did not identify the contract: %v\n%s", err, output)
			}
		})
	}
}

func TestEnvRejectionHelper(t *testing.T) {
	name := os.Getenv("PACKY_TESTPROCESS_REJECTION")
	if name == "" {
		return
	}
	additions := map[string][]string{
		"missing equals":           {"MALFORMED"},
		"empty key":                {"=value"},
		"NUL in key":               {"BAD\x00KEY=value"},
		"NUL in value":             {"BAD=value\x00tail"},
		"duplicate caller key":     {"DUPLICATE=first", "DUPLICATE=second"},
		"HOME":                     {"HOME=/replacement"},
		"TMPDIR":                   {"TMPDIR=/replacement"},
		"LANG":                     {"LANG=en_US.UTF-8"},
		"LC_ALL":                   {"LC_ALL=en_US.UTF-8"},
		"XDG_CACHE_HOME":           {"XDG_CACHE_HOME=/replacement"},
		"XDG_CONFIG_HOME":          {"XDG_CONFIG_HOME=/replacement"},
		"XDG_DATA_HOME":            {"XDG_DATA_HOME=/replacement"},
		"XDG_STATE_HOME":           {"XDG_STATE_HOME=/replacement"},
		"GO_TELEMETRY_CHILD":       {"GO_TELEMETRY_CHILD=0"},
		"GIT_CONFIG_GLOBAL":        {"GIT_CONFIG_GLOBAL=/replacement"},
		"GIT_CONFIG_NOSYSTEM":      {"GIT_CONFIG_NOSYSTEM=0"},
		"GIT_TERMINAL_PROMPT":      {"GIT_TERMINAL_PROMPT=1"},
		"GIT_CONFIG_COUNT":         {"GIT_CONFIG_COUNT=0"},
		"GIT_CONFIG_KEY pattern":   {"GIT_CONFIG_KEY_9=core.editor"},
		"GIT_CONFIG_VALUE pattern": {"GIT_CONFIG_VALUE_9=unsafe"},
	}[name]
	Env(t, additions...)
}

func TestGoOfflineEnvUsesFreshWritableCachesAndOneEscapedLocalProxy(t *testing.T) {
	t.Setenv("PATH", "/test/bin")
	outerModuleCache := filepath.Join(t.TempDir(), "module,cache|input")
	t.Setenv("GOMODCACHE", outerModuleCache)

	environment := GoOfflineEnv(t)
	values := environmentMap(t, environment)
	root := filepath.Dir(values["HOME"])
	want := map[string]string{
		"GOCACHE":     filepath.Join(root, "go", "build"),
		"GOMODCACHE":  filepath.Join(root, "go", "mod"),
		"GOPATH":      filepath.Join(root, "go", "path"),
		"GOENV":       "off",
		"GONOPROXY":   "none",
		"GOSUMDB":     "off",
		"GOTOOLCHAIN": "local",
		"GOVCS":       "*:off",
		"GOWORK":      "off",
	}
	for key, wantValue := range want {
		if got := values[key]; got != wantValue {
			t.Fatalf("%s = %q, want %q", key, got, wantValue)
		}
	}
	wantProxy := "file://" + strings.ReplaceAll(strings.ReplaceAll(filepath.ToSlash(outerModuleCache), ",", "%2C"), "|", "%7C") + "/cache/download"
	if got := values["GOPROXY"]; got != wantProxy {
		t.Fatalf("GOPROXY = %q, want %q", got, wantProxy)
	}
	if strings.ContainsAny(values["GOPROXY"], ",|") {
		t.Fatalf("GOPROXY contains a fallback separator: %q", values["GOPROXY"])
	}
	for _, key := range []string{"GOCACHE", "GOMODCACHE", "GOPATH"} {
		if !strings.HasPrefix(values[key], root+string(filepath.Separator)) {
			t.Fatalf("%s escaped test root: %q", key, values[key])
		}
		info, err := os.Stat(values[key])
		if err != nil {
			t.Fatalf("stat %s: %v", key, err)
		}
		if !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode = %v, want owner-only directory", key, info.Mode())
		}
	}
	if !sort.StringsAreSorted(environment) {
		t.Fatalf("GoOfflineEnv() is not sorted: %q", environment)
	}
}

func TestGoOfflineEnvRejectsEveryOwnedGoKey(t *testing.T) {
	for _, key := range []string{"GOCACHE", "GOMODCACHE", "GOPATH", "GOENV", "GONOPROXY", "GOPROXY", "GOSUMDB", "GOTOOLCHAIN", "GOVCS", "GOWORK"} {
		t.Run(key, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestGoOfflineEnvRejectionHelper$")
			command.Env = Env(t, "PACKY_TESTPROCESS_GO_REJECTION="+key)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatalf("GoOfflineEnv() accepted protected key %s:\n%s", key, output)
			}
			if !strings.Contains(string(output), "Go-offline test process environment") {
				t.Fatalf("GoOfflineEnv() rejection did not identify the contract: %v\n%s", err, output)
			}
		})
	}
}

func TestGoOfflineEnvRejectionHelper(t *testing.T) {
	key := os.Getenv("PACKY_TESTPROCESS_GO_REJECTION")
	if key == "" {
		return
	}
	GoOfflineEnv(t, key+"=replacement")
}

func TestGoOfflineEnvRejectsNonAbsoluteOrUncleanOuterModuleCache(t *testing.T) {
	for name, moduleCache := range map[string]string{
		"relative": "relative/module-cache",
		"unclean":  t.TempDir() + string(filepath.Separator) + "cache" + string(filepath.Separator) + ".." + string(filepath.Separator) + "module-cache",
	} {
		t.Run(name, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestGoOfflineEnvModuleCacheHelper$")
			command.Env = Env(t, "PACKY_TESTPROCESS_MODULE_CACHE=1", "GOMODCACHE="+moduleCache)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatalf("GoOfflineEnv() accepted GOMODCACHE %q:\n%s", moduleCache, output)
			}
			if !strings.Contains(string(output), "absolute clean path") {
				t.Fatalf("GoOfflineEnv() returned the wrong error: %v\n%s", err, output)
			}
		})
	}
}

func TestGoOfflineEnvModuleCacheHelper(t *testing.T) {
	if os.Getenv("PACKY_TESTPROCESS_MODULE_CACHE") == "" {
		return
	}
	GoOfflineEnv(t)
}

func TestGoOfflineEnvResolvesARealModuleOnlyFromTheLocalProxy(t *testing.T) {
	outerModuleCache := t.TempDir()
	writeProxyModule(t, outerModuleCache, "example.test/dependency", "v1.0.0", "package dependency\n\nconst Value = \"resolved-offline\"\n")
	before := snapshotFiles(t, outerModuleCache)
	t.Setenv("GOMODCACHE", outerModuleCache)

	project := t.TempDir()
	writeTestFile(t, filepath.Join(project, "go.mod"), "module example.test/application\n\ngo 1.25.0\n\nrequire example.test/dependency v1.0.0\n")
	writeTestFile(t, filepath.Join(project, "main.go"), "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n\t\"example.test/dependency\"\n)\n\nfunc main() { fmt.Printf(\"%s:%s\\n\", dependency.Value, os.Getenv(\"GO_TELEMETRY_CHILD\")) }\n")
	environment := GoOfflineEnv(t)
	values := environmentMap(t, environment)
	command := exec.Command(goToolPath(), "run", "-mod=mod", ".")
	command.Dir = project
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go run from local proxy: %v\n%s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != "resolved-offline:2" && !strings.HasSuffix(got, "\nresolved-offline:2") {
		t.Fatalf("go run output = %q, want local dependency and child telemetry mode on the final line", got)
	}
	if _, err := os.Stat(filepath.Join(values["GOMODCACHE"], "example.test", "dependency@v1.0.0", "dependency.go")); err != nil {
		t.Fatalf("fresh GOMODCACHE did not receive the dependency: %v", err)
	}
	if after := snapshotFiles(t, outerModuleCache); !reflect.DeepEqual(after, before) {
		t.Fatalf("outer module proxy changed:\nbefore=%#v\nafter=%#v", before, after)
	}
}

func TestGoOfflineEnvFailsLocallyWhenAModuleIsAbsent(t *testing.T) {
	outerModuleCache := t.TempDir()
	t.Setenv("GOMODCACHE", outerModuleCache)
	project := t.TempDir()
	writeTestFile(t, filepath.Join(project, "go.mod"), "module example.test/application\n\ngo 1.25.0\n\nrequire example.test/missing v1.0.0\n")
	writeTestFile(t, filepath.Join(project, "main.go"), "package main\n\nimport _ \"example.test/missing\"\n\nfunc main() {}\n")

	command := exec.Command(goToolPath(), "run", "-mod=mod", ".")
	command.Dir = project
	command.Env = GoOfflineEnv(t)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("go run unexpectedly resolved an absent local module:\n%s", output)
	}
	message := string(output)
	if !strings.Contains(message, "file:") || strings.Contains(message, "https://") {
		t.Fatalf("missing module did not fail solely at the local proxy: %v\n%s", err, output)
	}
}

func TestOnlyTestFilesImportTheTestProcessModule(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	moduleImport := strconv.Quote("github.com/yersonargotev/packy/internal/testprocess")
	if err := filepath.WalkDir(filepath.Join(repositoryRoot, "internal"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || filepath.Dir(path) == filepath.Join(repositoryRoot, "internal", "testprocess") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			if imported.Path.Value == moduleImport {
				t.Errorf("non-test Go file imports internal/testprocess: %s", path)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func writeProxyModule(t *testing.T, root, module, version, source string) {
	t.Helper()
	versionRoot := filepath.Join(root, "cache", "download", filepath.FromSlash(module), "@v")
	if err := os.MkdirAll(versionRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(versionRoot, "list"), version+"\n")
	writeTestFile(t, filepath.Join(versionRoot, version+".info"), "{\"Version\":\""+version+"\",\"Time\":\"2026-01-01T00:00:00Z\"}\n")
	writeTestFile(t, filepath.Join(versionRoot, version+".mod"), "module "+module+"\n\ngo 1.25.0\n")

	archive, err := os.Create(filepath.Join(versionRoot, version+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(archive)
	for name, contents := range map[string]string{
		module + "@" + version + "/go.mod":        "module " + module + "\n\ngo 1.25.0\n",
		module + "@" + version + "/dependency.go": source,
	} {
		entry, err := zipWriter.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func snapshotFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = string(data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func goToolPath() string {
	name := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(runtime.GOROOT(), "bin", name)
}

func environmentMap(t *testing.T, environment []string) map[string]string {
	t.Helper()
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			t.Fatalf("malformed environment entry %q", entry)
		}
		if _, exists := values[key]; exists {
			t.Fatalf("duplicate environment key %q", key)
		}
		values[key] = value
	}
	return values
}
