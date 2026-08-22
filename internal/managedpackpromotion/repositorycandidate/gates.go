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
	goCache, goModCache, goPath := currentGoCaches(ctx)
	environment := make([]string, 0, len(os.Environ())+12)
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if !sensitiveEnvironment(name) {
			environment = append(environment, value)
		}
	}
	environment = append(environment,
		"HOME="+filepath.Join(sandbox, "home"),
		"XDG_CONFIG_HOME="+filepath.Join(sandbox, "xdg"),
		"PACKY_VALIDATION_HOME="+filepath.Join(sandbox, "home"),
		"PACKY_VALIDATION_CONFIG_HOME="+filepath.Join(sandbox, "xdg"),
		"GOPROXY=off", "GONOPROXY=*", "GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		"GOCACHE="+goCache, "GOMODCACHE="+goModCache, "GOPATH="+goPath,
	)
	if err := os.MkdirAll(filepath.Join(sandbox, "home"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(sandbox, "xdg"), 0o755); err != nil {
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

func sensitiveEnvironment(name string) bool {
	upper := strings.ToUpper(name)
	if upper == "HOME" || upper == "XDG_CONFIG_HOME" || upper == "GOPROXY" || upper == "GONOPROXY" ||
		upper == "PACKY_VALIDATION_HOME" || upper == "PACKY_VALIDATION_CONFIG_HOME" ||
		upper == "HTTP_PROXY" || upper == "HTTPS_PROXY" || upper == "ALL_PROXY" || upper == "NO_PROXY" ||
		upper == "SSH_AUTH_SOCK" || upper == "SSH_ASKPASS" || upper == "GIT_ASKPASS" ||
		upper == "GIT_SSH" || upper == "GIT_SSH_COMMAND" || upper == "GIT_CREDENTIAL_HELPER" ||
		strings.HasPrefix(upper, "GIT_CONFIG_") || strings.HasPrefix(upper, "GH_") || strings.HasPrefix(upper, "GITHUB_") ||
		strings.HasPrefix(upper, "AWS_") || strings.HasPrefix(upper, "AZURE_") || strings.HasPrefix(upper, "GOOGLE_") {
		return true
	}
	return strings.HasSuffix(upper, "_TOKEN") || strings.HasSuffix(upper, "_SECRET") || strings.HasSuffix(upper, "_PASSWORD") || strings.HasSuffix(upper, "_CREDENTIAL")
}

func currentGoCaches(ctx context.Context) (string, string, string) {
	command := exec.CommandContext(ctx, "go", "env", "GOCACHE", "GOMODCACHE", "GOPATH")
	output, err := command.Output()
	if err != nil {
		return os.Getenv("GOCACHE"), os.Getenv("GOMODCACHE"), os.Getenv("GOPATH")
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 3 {
		return os.Getenv("GOCACHE"), os.Getenv("GOMODCACHE"), os.Getenv("GOPATH")
	}
	return lines[0], lines[1], lines[2]
}
