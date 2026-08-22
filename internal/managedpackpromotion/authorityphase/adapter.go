// Package authorityphase separates Managed Pack parsing and admission from
// the only process authorized to mutate GitHub proposal state.
package authorityphase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

const (
	PrepareModeArgument = "--packy-internal-prepublication-worker"
	PublishModeArgument = "--packy-internal-publication-worker"
	phaseTimeout        = 20 * time.Minute
	maxProcessOutput    = 64 << 10
)

type Adapter struct {
	executable string
	ambient    []string
	runner     processRunner
}

func New(executable string) Adapter {
	return newWithRunner(executable, os.Environ(), execProcessRunner{})
}

func newWithRunner(executable string, ambient []string, runner processRunner) Adapter {
	return Adapter{executable: executable, ambient: append([]string(nil), ambient...), runner: runner}
}

func (adapter Adapter) Promote(ctx context.Context, request managedpackpromotion.Request) (result managedpackpromotion.Result, resultErr error) {
	if err := validateExecutable(adapter.executable); err != nil {
		return managedpackpromotion.Result{}, err
	}
	if adapter.runner == nil {
		return managedpackpromotion.Result{}, errors.New("promotion process runner is required")
	}
	protocolRoot, err := os.MkdirTemp("", "packy-promotion-authority-")
	if err != nil {
		return managedpackpromotion.Result{}, fmt.Errorf("create promotion authority protocol root: %w", err)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(protocolRoot); cleanupErr != nil {
			result = managedpackpromotion.Result{}
			resultErr = errors.Join(resultErr, fmt.Errorf("clean up promotion authority protocol root: %w", cleanupErr))
		}
	}()
	if err := os.Chmod(protocolRoot, 0o700); err != nil {
		return managedpackpromotion.Result{}, err
	}
	prepublicationEnv, err := prepublicationEnvironment(ctx, protocolRoot, adapter.ambient)
	if err != nil {
		return managedpackpromotion.Result{}, err
	}
	sanitizedRepository := filepath.Join(protocolRoot, "repository")
	if err := snapshotRepository(ctx, request.RepositoryRoot, sanitizedRepository, prepublicationEnv); err != nil {
		return managedpackpromotion.Result{}, err
	}
	request.RepositoryRoot = sanitizedRepository

	prepare, err := newPrepareRequest(request)
	if err != nil {
		return managedpackpromotion.Result{}, err
	}
	prepareRequestPath := filepath.Join(protocolRoot, "prepare-request.json")
	prepareResponsePath := filepath.Join(protocolRoot, "prepare-response.json")
	if err := writePrepareRequest(prepareRequestPath, prepare); err != nil {
		return managedpackpromotion.Result{}, fmt.Errorf("write prepublication request: %w", err)
	}
	if err := adapter.runPhase(ctx, processInvocation{Mode: PrepareModeArgument, RequestPath: prepareRequestPath, ResponsePath: prepareResponsePath, Environment: prepublicationEnv, Directory: protocolRoot}); err != nil {
		return managedpackpromotion.Result{}, fmt.Errorf("run prepublication worker: %w", err)
	}
	prepared, err := readPrepareResponse(prepareResponsePath, prepare)
	if err != nil {
		return managedpackpromotion.Result{}, fmt.Errorf("read prepublication response: %w", err)
	}
	switch prepared.Status {
	case prepareResult:
		if err := validateTerminalResult(prepared.Result); err != nil || prepared.Candidate != (managedpackpromotion.Candidate{}) {
			if err == nil {
				err = errors.New("terminal prepublication response contains a candidate")
			}
			return managedpackpromotion.Result{}, err
		}
		return prepared.Result, nil
	case prepareCandidate:
		if prepared.Result != (managedpackpromotion.Result{}) {
			return managedpackpromotion.Result{}, errors.New("candidate prepublication response contains a terminal result")
		}
		if err := validateStagedCandidate(protocolRoot, request.Coordinate, prepared.Candidate); err != nil {
			return managedpackpromotion.Result{}, err
		}
	default:
		return managedpackpromotion.Result{}, fmt.Errorf("unsupported prepublication response status %q", prepared.Status)
	}

	publish, err := newPublishRequest(prepared.Candidate)
	if err != nil {
		return managedpackpromotion.Result{}, err
	}
	publishRequestPath := filepath.Join(protocolRoot, "publish-request.json")
	publishResponsePath := filepath.Join(protocolRoot, "publish-response.json")
	if err := writePublishRequest(publishRequestPath, publish); err != nil {
		return managedpackpromotion.Result{}, fmt.Errorf("write publication request: %w", err)
	}
	if err := adapter.runPhase(ctx, processInvocation{Mode: PublishModeArgument, RequestPath: publishRequestPath, ResponsePath: publishResponsePath, Environment: publicationEnvironment(adapter.ambient), Directory: protocolRoot}); err != nil {
		return managedpackpromotion.Result{}, fmt.Errorf("run publication worker: %w", err)
	}
	published, err := readPublishResponse(publishResponsePath, publish)
	if err != nil {
		return managedpackpromotion.Result{}, fmt.Errorf("read publication response: %w", err)
	}
	if err := validatePublishedResult(published.Result); err != nil {
		return managedpackpromotion.Result{}, err
	}
	return published.Result, nil
}

func (adapter Adapter) runPhase(ctx context.Context, invocation processInvocation) error {
	phaseContext, cancel := context.WithTimeout(ctx, phaseTimeout)
	defer cancel()
	invocation.Executable = adapter.executable
	output := &boundedBuffer{limit: maxProcessOutput}
	invocation.Output = output
	if err := adapter.runner.Run(phaseContext, invocation); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		if contextErr := phaseContext.Err(); contextErr != nil {
			return fmt.Errorf("worker exceeded its bounded execution window: %w", contextErr)
		}
		detail := strings.TrimSpace(output.String())
		if detail != "" {
			return fmt.Errorf("%w: %s", err, detail)
		}
		return err
	}
	if output.overflow {
		return errors.New("promotion worker output exceeded its bound")
	}
	if output.buffer.Len() != 0 {
		return errors.New("promotion worker wrote unexpected process output")
	}
	return nil
}

type processInvocation struct {
	Executable   string
	Mode         string
	RequestPath  string
	ResponsePath string
	Environment  []string
	Directory    string
	Output       *boundedBuffer
}

type processRunner interface {
	Run(context.Context, processInvocation) error
}
type processRunnerFunc func(context.Context, processInvocation) error

func (run processRunnerFunc) Run(ctx context.Context, invocation processInvocation) error {
	return run(ctx, invocation)
}

type execProcessRunner struct{}

func (execProcessRunner) Run(ctx context.Context, invocation processInvocation) error {
	command := exec.CommandContext(ctx, invocation.Executable, invocation.Mode, invocation.RequestPath, invocation.ResponsePath)
	command.Dir = invocation.Directory
	command.Env = append([]string(nil), invocation.Environment...)
	command.Stdin = nil
	command.Stdout, command.Stderr = invocation.Output, invocation.Output
	return command.Run()
}

func validateExecutable(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("promotion executable must be an absolute clean path")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o111 == 0 {
		return errors.New("promotion executable must be an executable regular file and not a symlink")
	}
	return nil
}

func prepublicationEnvironment(ctx context.Context, root string, ambient []string) ([]string, error) {
	values := map[string]string{
		"HOME": filepath.Join(root, "home"), "TMPDIR": filepath.Join(root, "tmp"),
		"XDG_CACHE_HOME": filepath.Join(root, "xdg-cache"), "XDG_CONFIG_HOME": filepath.Join(root, "xdg-config"),
		"XDG_DATA_HOME": filepath.Join(root, "xdg-data"), "XDG_STATE_HOME": filepath.Join(root, "xdg-state"),
		"LANG": "C", "LC_ALL": "C", "GIT_CONFIG_NOSYSTEM": "1", "GIT_TERMINAL_PROMPT": "0", "GOENV": "off", "GOTELEMETRY": "off", "GOTOOLCHAIN": "local",
	}
	for _, path := range []string{values["HOME"], values["TMPDIR"], values["XDG_CACHE_HOME"], values["XDG_CONFIG_HOME"], values["XDG_DATA_HOME"], values["XDG_STATE_HOME"]} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return nil, fmt.Errorf("create prepublication environment directory: %w", err)
		}
	}
	allowed := map[string]bool{"PATH": true, "GOROOT": true, "GOCACHE": true, "GOMODCACHE": true, "GOPATH": true, "SSL_CERT_FILE": true, "SSL_CERT_DIR": true}
	for _, entry := range ambient {
		key, value, ok := strings.Cut(entry, "=")
		if ok && allowed[key] && value != "" {
			values[key] = value
		}
	}
	if values["PATH"] == "" {
		values["PATH"] = "/usr/local/go/bin:/usr/bin:/bin"
	}
	goValues, err := currentGoEnvironment(ctx, ambient)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"GOCACHE", "GOMODCACHE", "GOPATH", "GOROOT"} {
		if values[key] == "" {
			values[key] = goValues[key]
		}
		if values[key] == "" || !filepath.IsAbs(values[key]) || filepath.Clean(values[key]) != values[key] {
			return nil, fmt.Errorf("Go environment %s must be an absolute clean path", key)
		}
	}
	return sortedEnvironment(values), nil
}

func currentGoEnvironment(ctx context.Context, ambient []string) (map[string]string, error) {
	command := exec.CommandContext(ctx, "go", "env", "-json", "GOCACHE", "GOMODCACHE", "GOPATH", "GOROOT")
	command.Env = append([]string(nil), ambient...)
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("resolve Go cache environment for prepublication: %w", err)
	}
	if len(output) > maxProcessOutput {
		return nil, errors.New("Go cache environment output exceeded its bound")
	}
	values := map[string]string{}
	if err := json.Unmarshal(output, &values); err != nil {
		return nil, fmt.Errorf("decode Go cache environment: %w", err)
	}
	return values, nil
}

var publicationKeys = map[string]bool{
	"PATH": true, "HOME": true, "TMPDIR": true, "LANG": true, "LC_ALL": true, "LC_CTYPE": true,
	"XDG_CONFIG_HOME": true, "GH_CONFIG_DIR": true, "GH_TOKEN": true, "GITHUB_TOKEN": true,
	"GH_ENTERPRISE_TOKEN": true, "GITHUB_ENTERPRISE_TOKEN": true, "GH_HOST": true, "GH_REPO": true,
	"SSH_AUTH_SOCK": true, "SSL_CERT_FILE": true, "SSL_CERT_DIR": true,
}

func publicationEnvironment(ambient []string) []string {
	values := map[string]string{"GCM_INTERACTIVE": "Never", "GH_PROMPT_DISABLED": "1", "GIT_TERMINAL_PROMPT": "0"}
	for _, entry := range ambient {
		key, value, ok := strings.Cut(entry, "=")
		if ok && publicationKeys[key] {
			values[key] = value
		}
	}
	return sortedEnvironment(values)
}

func sortedEnvironment(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

func snapshotRepository(ctx context.Context, source, destination string, environment []string) error {
	head, err := commandText(ctx, source, environment, "git", "rev-parse", "HEAD^{commit}")
	if err != nil {
		return fmt.Errorf("resolve Packy HEAD: %w", err)
	}
	base, err := commandText(ctx, source, environment, "git", "rev-parse", "refs/remotes/origin/main^{commit}")
	if err != nil {
		return fmt.Errorf("resolve Packy origin/main: %w", err)
	}
	remote, err := commandText(ctx, source, environment, "git", "remote", "get-url", "origin")
	if err != nil {
		return fmt.Errorf("resolve Packy origin URL: %w", err)
	}
	remote, err = sanitizeRemote(remote)
	if err != nil {
		return err
	}
	if _, err := commandText(ctx, "", environment, "git", "clone", "--local", "--no-hardlinks", "--no-checkout", source, destination); err != nil {
		return fmt.Errorf("clone sanitized Packy repository: %w", err)
	}
	commands := [][]string{{"checkout", "--detach", head}, {"update-ref", "refs/remotes/origin/main", base}, {"remote", "set-url", "origin", remote}}
	for _, arguments := range commands {
		if _, err := commandText(ctx, destination, environment, "git", arguments...); err != nil {
			return fmt.Errorf("sanitize Packy repository: %w", err)
		}
	}
	return nil
}

func sanitizeRemote(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("Packy origin URL is empty")
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return "", errors.New("Packy origin URL is malformed")
		}
		if parsed.Scheme == "ssh" {
			parsed.User = url.User("git")
		} else {
			parsed.User = nil
		}
		parsed.RawQuery, parsed.Fragment = "", ""
		return parsed.String(), nil
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	left, path, ok := strings.Cut(value, ":")
	if !ok || path == "" || strings.ContainsAny(left, "/\\\r\n") || strings.ContainsAny(path, "\r\n") {
		return "", errors.New("Packy origin URL is neither an absolute local path nor a valid Git SSH remote")
	}
	host := left
	if _, candidate, found := strings.Cut(left, "@"); found {
		host = candidate
	}
	if host == "" {
		return "", errors.New("Packy Git SSH remote has no host")
	}
	return "git@" + host + ":" + path, nil
}

func commandText(ctx context.Context, directory string, environment []string, executable string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir, command.Env = directory, append([]string(nil), environment...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", executable, strings.Join(arguments, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func validateStagedCandidate(root string, coordinate managedpackpromotion.Coordinate, candidate managedpackpromotion.Candidate) error {
	wantRoot := filepath.Join(root, candidateDirectoryName)
	if candidate.RepositoryRoot != wantRoot || !pathWithin(root, candidate.RepositoryRoot) {
		return errors.New("staged candidate root is outside the protocol root")
	}
	info, err := os.Lstat(candidate.RepositoryRoot)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("staged candidate root is not a real directory")
	}
	if candidate.Coordinate != coordinate || strings.TrimSpace(candidate.ID) == "" || strings.TrimSpace(candidate.Summary) == "" || strings.TrimSpace(candidate.Project) == "" || strings.TrimSpace(candidate.Branch) == "" || !shaPattern.MatchString(candidate.BaseSHA) || !shaPattern.MatchString(candidate.HeadSHA) || !shaPattern.MatchString(candidate.ResultTreeSHA) {
		return errors.New("staged candidate is incomplete or does not match the request")
	}
	return nil
}

func validateTerminalResult(result managedpackpromotion.Result) error {
	switch result.Status {
	case managedpackpromotion.StatusNoChange:
		if strings.TrimSpace(result.Reason) == "" || result.Rejection != nil || result.Proposal != nil {
			return errors.New("invalid no-change prepublication result")
		}
	case managedpackpromotion.StatusRejected:
		if result.Rejection == nil || result.Rejection.Gate == "" || strings.TrimSpace(result.Rejection.Reason) == "" || result.Reason != "" || result.Proposal != nil {
			return errors.New("invalid rejected prepublication result")
		}
	default:
		return errors.New("prepublication worker returned a non-terminal result")
	}
	return nil
}

func resultForPublication(publication managedpackpromotion.Publication) (managedpackpromotion.Result, error) {
	if publication.Proposal == nil {
		if strings.TrimSpace(publication.NoChangeReason) == "" {
			return managedpackpromotion.Result{}, errors.New("publisher returned neither proposal nor no-change")
		}
		return managedpackpromotion.Result{Status: managedpackpromotion.StatusNoChange, Reason: publication.NoChangeReason}, nil
	}
	if publication.NoChangeReason != "" || publication.Proposal.Number <= 0 || publication.Proposal.URL == "" || publication.Proposal.Branch == "" || !shaPattern.MatchString(publication.Proposal.HeadSHA) {
		return managedpackpromotion.Result{}, errors.New("publisher returned an invalid proposal")
	}
	return managedpackpromotion.Result{Status: managedpackpromotion.StatusProposal, Proposal: publication.Proposal}, nil
}

func validatePublishedResult(result managedpackpromotion.Result) error {
	if result.Status == managedpackpromotion.StatusRejected {
		if result.Rejection == nil || result.Rejection.Gate == "" || strings.TrimSpace(result.Rejection.Reason) == "" || result.Reason != "" || result.Proposal != nil {
			return errors.New("publication worker returned an invalid rejection")
		}
		return nil
	}
	publication := managedpackpromotion.Publication{Proposal: result.Proposal, NoChangeReason: result.Reason}
	want, err := resultForPublication(publication)
	if err != nil {
		return err
	}
	if want.Status != result.Status {
		return errors.New("publication worker returned an inconsistent result")
	}
	return nil
}

type boundedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	provided := len(data)
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining < len(data) {
		buffer.overflow = true
		if remaining < 0 {
			remaining = 0
		}
		data = data[:remaining]
	}
	_, err := buffer.buffer.Write(data)
	return provided, err
}
func (buffer *boundedBuffer) String() string { return buffer.buffer.String() }

var _ interface {
	Promote(context.Context, managedpackpromotion.Request) (managedpackpromotion.Result, error)
} = Adapter{}
