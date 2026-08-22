package repositorycandidate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

type productionGates struct{}

func (productionGates) GenerateDocs(ctx context.Context, repositoryRoot string) error {
	return runSanitized(ctx, repositoryRoot, "go", "run", "./internal/tools/packdocs")
}

func (productionGates) ValidateResources(ctx context.Context, repositoryRoot string) error {
	bundleRoot := filepath.Join(repositoryRoot, "bundle")
	if err := capabilitypack.ValidatePortableContent(bundleRoot); err != nil {
		return err
	}
	catalog, err := capabilitypack.Discover(ctx, bundleRoot)
	if err != nil {
		return err
	}
	if _, err := catalog.ListDetails(ctx); err != nil {
		return err
	}
	return runSanitized(ctx, repositoryRoot, "go", "test", "./internal/capabilitypack")
}

func (productionGates) ValidateSuite(ctx context.Context, repositoryRoot string) error {
	return runSanitized(ctx, repositoryRoot, "./scripts/validate-packy.sh")
}

func runSanitized(ctx context.Context, repositoryRoot, name string, arguments ...string) error {
	sandbox, err := os.MkdirTemp("", "packy-promotion-gate-*")
	if err != nil {
		return fmt.Errorf("create gate sandbox: %w", err)
	}
	defer os.RemoveAll(sandbox)
	home := filepath.Join(sandbox, "home")
	config := filepath.Join(sandbox, "xdg", "config")
	cache := filepath.Join(sandbox, "xdg", "cache")
	temporary := filepath.Join(sandbox, "tmp")
	for _, directory := range []string{home, config, cache, temporary} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return err
		}
	}
	environment, err := gateEnvironment(ctx, home, config, cache, temporary)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, name, arguments...)
	command.Dir = repositoryRoot
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(arguments, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func gateEnvironment(ctx context.Context, home, config, cache, temporary string) ([]string, error) {
	goCache, goModCache, goPath, err := currentGoCaches(ctx)
	if err != nil {
		return nil, err
	}
	return []string{
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GOCACHE=" + goCache,
		"GOMODCACHE=" + goModCache,
		"GONOPROXY=*",
		"GOPATH=" + goPath,
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOTOOLCHAIN=local",
		"GOVCS=*:off",
		"HOME=" + home,
		"LANG=C",
		"LC_ALL=C",
		"PACKY_VALIDATION_CONFIG_HOME=" + config,
		"PACKY_VALIDATION_HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"TMPDIR=" + temporary,
		"XDG_CACHE_HOME=" + cache,
		"XDG_CONFIG_HOME=" + config,
	}, nil
}

func currentGoCaches(ctx context.Context) (string, string, string, error) {
	goExecutable := filepath.Join(runtime.GOROOT(), "bin", "go")
	if !filepath.IsAbs(goExecutable) {
		return "", "", "", errors.New("Go executable path is not absolute")
	}
	environment := []string{
		"GOENV=off",
		"GOTELEMETRY=off",
		"GOTOOLCHAIN=local",
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
		"XDG_CONFIG_HOME=" + filepath.Join(os.TempDir(), "packy-promotion-go-env"),
	}
	for _, name := range []string{"GOCACHE", "GOMODCACHE", "GOPATH"} {
		if value := os.Getenv(name); value != "" {
			if !filepath.IsAbs(value) || filepath.Clean(value) != value {
				return "", "", "", fmt.Errorf("%s must be an absolute clean path", name)
			}
			environment = append(environment, name+"="+value)
		}
	}
	command := exec.CommandContext(ctx, goExecutable, "env", "GOCACHE", "GOMODCACHE", "GOPATH")
	command.Env = environment
	output, err := command.Output()
	if err != nil {
		return "", "", "", fmt.Errorf("resolve preserved Go caches: %w", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(lines) != 3 {
		return "", "", "", errors.New("go env returned an incomplete Go cache configuration")
	}
	for index := range lines {
		lines[index] = strings.TrimSuffix(lines[index], "\r")
		if !filepath.IsAbs(lines[index]) || filepath.Clean(lines[index]) != lines[index] {
			return "", "", "", errors.New("go env returned a non-absolute Go cache path")
		}
	}
	return lines[0], lines[1], lines[2], nil
}
