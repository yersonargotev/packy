package testprocess

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// Env returns an isolated environment for a subprocess owned by test code.
func Env(t testing.TB, additions ...string) []string {
	t.Helper()
	explicit, err := validateAdditions(additions, nil)
	if err != nil {
		t.Fatalf("construct test process environment: %v", err)
	}
	values, _ := baseEnvironment(t)
	for key, value := range explicit {
		values[key] = value
	}
	return sortedEnvironment(values)
}

// GoOfflineEnv returns Env extended with fresh writable Go caches and one
// read-only local module proxy sourced from the outer module download cache.
func GoOfflineEnv(t testing.TB, additions ...string) []string {
	t.Helper()
	explicit, err := validateAdditions(additions, goOfflineKeys)
	if err != nil {
		t.Fatalf("construct Go-offline test process environment: %v", err)
	}
	sourceModuleCache, err := outerModuleCache()
	if err != nil {
		t.Fatalf("construct Go-offline test process environment: %v", err)
	}

	values, root := baseEnvironment(t)
	goDirectories := map[string]string{
		"GOCACHE":    filepath.Join(root, "go", "build"),
		"GOMODCACHE": filepath.Join(root, "go", "mod"),
		"GOPATH":     filepath.Join(root, "go", "path"),
	}
	for _, directory := range goDirectories {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("construct Go-offline test process environment: create %s: %v", directory, err)
		}
	}
	t.Cleanup(func() {
		if err := makeOwnerWritable(filepath.Join(root, "go")); err != nil && !os.IsNotExist(err) {
			t.Errorf("clean Go-offline test process environment: %v", err)
		}
	})
	for key, value := range goDirectories {
		values[key] = value
	}
	values["GOENV"] = "off"
	values["GONOPROXY"] = "none"
	values["GOPROXY"] = localModuleProxy(sourceModuleCache)
	values["GOSUMDB"] = "off"
	values["GOTOOLCHAIN"] = "local"
	values["GOVCS"] = "*:off"
	values["GOWORK"] = "off"
	for key, value := range explicit {
		values[key] = value
	}
	return sortedEnvironment(values)
}

func baseEnvironment(t testing.TB) (map[string]string, string) {
	t.Helper()
	path := os.Getenv("PATH")
	if path == "" {
		t.Fatal("construct test process environment: PATH is empty")
	}

	root := t.TempDir()
	directories := map[string]string{
		"HOME":            filepath.Join(root, "home"),
		"TMPDIR":          filepath.Join(root, "tmp"),
		"XDG_CACHE_HOME":  filepath.Join(root, "xdg", "cache"),
		"XDG_CONFIG_HOME": filepath.Join(root, "xdg", "config"),
		"XDG_DATA_HOME":   filepath.Join(root, "xdg", "data"),
		"XDG_STATE_HOME":  filepath.Join(root, "xdg", "state"),
	}
	for _, directory := range directories {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("construct test process environment: create %s: %v", directory, err)
		}
	}

	values := map[string]string{
		"GIT_CONFIG_COUNT":    "2",
		"GIT_CONFIG_GLOBAL":   os.DevNull,
		"GIT_CONFIG_KEY_0":    "maintenance.auto",
		"GIT_CONFIG_KEY_1":    "gc.auto",
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_CONFIG_VALUE_0":  "false",
		"GIT_CONFIG_VALUE_1":  "0",
		"GIT_TERMINAL_PROMPT": "0",
		"GO_TELEMETRY_CHILD":  "2",
		"LANG":                "C",
		"LC_ALL":              "C",
		"PATH":                path,
	}
	for key, directory := range directories {
		values[key] = directory
	}
	return values, root
}

func sortedEnvironment(values map[string]string) []string {
	environment := make([]string, 0, len(values))
	for key, value := range values {
		environment = append(environment, key+"="+value)
	}
	sort.Strings(environment)
	return environment
}

var goOfflineKeys = map[string]struct{}{
	"GOCACHE": {}, "GOMODCACHE": {}, "GOPATH": {}, "GOENV": {}, "GONOPROXY": {},
	"GOPROXY": {}, "GOSUMDB": {}, "GOTOOLCHAIN": {}, "GOVCS": {}, "GOWORK": {},
}

func outerModuleCache() (string, error) {
	if configured := os.Getenv("GOMODCACHE"); configured != "" {
		return validateModuleCache(configured)
	}

	goExecutable := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goExecutable += ".exe"
	}
	// HOME is intentionally the outer value only for deriving Go's default
	// module cache. GOENV and child telemetry remain disabled, and this value is
	// never returned to the sandboxed child.
	environment := []string{
		"GOENV=off",
		"GOTOOLCHAIN=local",
		"GO_TELEMETRY_CHILD=2",
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
	}
	if goPath := os.Getenv("GOPATH"); goPath != "" {
		environment = append(environment, "GOPATH="+goPath)
	}
	sort.Strings(environment)
	command := exec.Command(goExecutable, "env", "GOMODCACHE")
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resolve outer GOMODCACHE with %s: %w: %s", goExecutable, err, strings.TrimSpace(string(output)))
	}
	moduleCache := strings.TrimSuffix(string(output), "\n")
	moduleCache = strings.TrimSuffix(moduleCache, "\r")
	if strings.ContainsAny(moduleCache, "\r\n") {
		return "", fmt.Errorf("resolve outer GOMODCACHE: Go returned multiple lines")
	}
	return validateModuleCache(moduleCache)
}

func validateModuleCache(moduleCache string) (string, error) {
	if moduleCache == "" || !filepath.IsAbs(moduleCache) || filepath.Clean(moduleCache) != moduleCache {
		return "", fmt.Errorf("outer GOMODCACHE %q must be an absolute clean path", moduleCache)
	}
	return moduleCache, nil
}

func localModuleProxy(moduleCache string) string {
	proxyPath := filepath.ToSlash(filepath.Join(moduleCache, "cache", "download"))
	if volume := filepath.VolumeName(moduleCache); volume != "" && !strings.HasPrefix(proxyPath, "/") {
		proxyPath = "/" + proxyPath
	}
	proxy := (&url.URL{Scheme: "file", Path: proxyPath}).String()
	proxy = strings.ReplaceAll(proxy, ",", "%2C")
	return strings.ReplaceAll(proxy, "|", "%7C")
}

func makeOwnerWritable(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		mode := os.FileMode(0o600)
		if entry.IsDir() {
			mode = 0o700
		}
		return os.Chmod(path, mode)
	})
}

func validateAdditions(additions []string, additionallyProtected map[string]struct{}) (map[string]string, error) {
	values := make(map[string]string, len(additions))
	for _, addition := range additions {
		if strings.ContainsRune(addition, '\x00') {
			return nil, fmt.Errorf("addition contains NUL byte")
		}
		key, value, ok := strings.Cut(addition, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("malformed addition %q", addition)
		}
		if baseKeyProtected(key) {
			return nil, fmt.Errorf("addition overrides protected key %q", key)
		}
		if _, protected := additionallyProtected[key]; protected {
			return nil, fmt.Errorf("addition overrides protected key %q", key)
		}
		if _, duplicate := values[key]; duplicate {
			return nil, fmt.Errorf("duplicate addition key %q", key)
		}
		values[key] = value
	}
	return values, nil
}

func baseKeyProtected(key string) bool {
	if strings.HasPrefix(key, "GIT_CONFIG_") {
		return true
	}
	switch key {
	case "HOME", "TMPDIR", "LANG", "LC_ALL",
		"XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME",
		"GO_TELEMETRY_CHILD", "GIT_TERMINAL_PROMPT":
		return true
	default:
		return false
	}
}
