package repositorycandidate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	environment := gateEnvironment(ctx, home, config, cache, temporary)
	command := exec.CommandContext(ctx, name, arguments...)
	command.Dir = repositoryRoot
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(arguments, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func gateEnvironment(ctx context.Context, home, config, cache, temporary string) []string {
	goCache, goModCache, goPath := currentGoCaches(ctx)
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
	}
}

func currentGoCaches(ctx context.Context) (string, string, string) {
	command := exec.CommandContext(ctx, "go", "env", "GOCACHE", "GOMODCACHE", "GOPATH")
	command.Env = []string{
		"GOENV=off",
		"GOTELEMETRY=off",
		"GOTOOLCHAIN=local",
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
		"XDG_CONFIG_HOME=" + filepath.Join(os.TempDir(), "packy-promotion-go-env"),
	}
	output, err := command.Output()
	if err != nil {
		return os.Getenv("GOCACHE"), os.Getenv("GOMODCACHE"), os.Getenv("GOPATH")
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 3 || !filepath.IsAbs(lines[0]) || !filepath.IsAbs(lines[1]) || !filepath.IsAbs(lines[2]) {
		return os.Getenv("GOCACHE"), os.Getenv("GOMODCACHE"), os.Getenv("GOPATH")
	}
	return lines[0], lines[1], lines[2]
}
