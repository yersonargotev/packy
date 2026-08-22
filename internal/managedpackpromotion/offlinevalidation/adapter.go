// Package offlinevalidation runs Managed Pack validation in one isolated,
// credential-free subprocess over already acquired local trees.
package offlinevalidation

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

const workerTimeout = 5 * time.Minute

// ModeArgument is the private command argument that routes execution to Run.
const ModeArgument = "--packy-internal-offline-validation-worker"

// Adapter implements managedpackpromotion.OfflineValidator with one exact
// executable. It never resolves the executable through a shell or PATH.
type Adapter struct {
	executable string
	runner     processRunner
}

// New constructs the production subprocess adapter.
func New(executable string) Adapter {
	return newWithRunner(executable, execProcessRunner{})
}

func newWithRunner(executable string, runner processRunner) Adapter {
	return Adapter{executable: executable, runner: runner}
}

// Validate validates only the local trees sealed into acquisition.
func (adapter Adapter) Validate(ctx context.Context, acquisition managedpackpromotion.Acquisition) (managedpack.Validation, error) {
	if err := ctx.Err(); err != nil {
		return managedpack.Validation{}, err
	}
	workerContext, cancelWorker := context.WithTimeout(ctx, workerTimeout)
	defer cancelWorker()
	if err := validateExecutable(adapter.executable); err != nil {
		return managedpack.Validation{}, err
	}
	if adapter.runner == nil {
		return managedpack.Validation{}, errors.New("offline validation process runner is required")
	}
	if err := validateAcquisitionRoots(acquisition.ProjectRoot, acquisition.OriginRoots); err != nil {
		return managedpack.Validation{}, fmt.Errorf("validate acquired local roots: %w", err)
	}

	protocolRoot, err := os.MkdirTemp("", "packy-offline-validation-")
	if err != nil {
		return managedpack.Validation{}, fmt.Errorf("create offline validation protocol directory: %w", err)
	}
	defer os.RemoveAll(protocolRoot)
	environment, err := isolatedEnvironment(protocolRoot)
	if err != nil {
		return managedpack.Validation{}, err
	}
	request, requestPath, responsePath, err := createWorkerRequest(protocolRoot, acquisition)
	if err != nil {
		return managedpack.Validation{}, err
	}

	stdout := newCappedBuffer(maxProcessOutputBytes)
	stderr := newCappedBuffer(maxProcessOutputBytes)
	invocation := processInvocation{
		Executable: adapter.executable,
		Args:       []string{ModeArgument, requestPath, responsePath},
		Env:        environment,
		Dir:        protocolRoot,
		Stdout:     stdout,
		Stderr:     stderr,
	}
	if err := adapter.runner.Run(workerContext, invocation); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return managedpack.Validation{}, contextErr
		}
		if contextErr := workerContext.Err(); contextErr != nil {
			return managedpack.Validation{}, fmt.Errorf("offline validation worker exceeded its resource-bounded execution window: %w", contextErr)
		}
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return managedpack.Validation{}, fmt.Errorf("run offline validation worker: %w: %s", err, detail)
		}
		return managedpack.Validation{}, fmt.Errorf("run offline validation worker: %w", err)
	}
	if stdout.Len() != 0 || stdout.Overflowed() {
		return managedpack.Validation{}, errors.New("offline validation worker wrote unexpected standard output")
	}
	response, err := readWorkerResponse(responsePath, request)
	if err != nil {
		return managedpack.Validation{}, fmt.Errorf("read offline validation response: %w", err)
	}
	switch response.Status {
	case responseAccepted:
		return response.Validation, nil
	case responseRejected:
		return managedpack.Validation{}, managedpackpromotion.Reject(response.Gate, response.Reason)
	default:
		return managedpack.Validation{}, fmt.Errorf("offline validation response has unsupported status %q", response.Status)
	}
}

func validateExecutable(executable string) error {
	if strings.TrimSpace(executable) == "" || !filepath.IsAbs(executable) || filepath.Clean(executable) != executable {
		return errors.New("offline validation executable must be an absolute clean path")
	}
	info, err := os.Lstat(executable)
	if err != nil {
		return fmt.Errorf("inspect offline validation executable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return errors.New("offline validation executable must be an executable regular file and not a symlink")
	}
	return nil
}

func isolatedEnvironment(root string) ([]string, error) {
	paths := map[string]string{
		"HOME":            filepath.Join(root, "home"),
		"TMPDIR":          filepath.Join(root, "tmp"),
		"XDG_CACHE_HOME":  filepath.Join(root, "xdg", "cache"),
		"XDG_CONFIG_HOME": filepath.Join(root, "xdg", "config"),
		"XDG_DATA_HOME":   filepath.Join(root, "xdg", "data"),
		"XDG_STATE_HOME":  filepath.Join(root, "xdg", "state"),
	}
	for name, path := range paths {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return nil, fmt.Errorf("create isolated %s directory: %w", name, err)
		}
		if err := requireRealDirectory(path); err != nil {
			return nil, fmt.Errorf("inspect isolated %s directory: %w", name, err)
		}
	}
	values := map[string]string{
		"HOME": paths["HOME"], "LANG": "C", "LC_ALL": "C", "PATH": "/usr/bin:/bin",
		"TMPDIR": paths["TMPDIR"], "XDG_CACHE_HOME": paths["XDG_CACHE_HOME"],
		"XDG_CONFIG_HOME": paths["XDG_CONFIG_HOME"], "XDG_DATA_HOME": paths["XDG_DATA_HOME"],
		"XDG_STATE_HOME": paths["XDG_STATE_HOME"],
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	return environment, nil
}

func createWorkerRequest(root string, acquisition managedpackpromotion.Acquisition) (workerRequest, string, string, error) {
	token, err := randomIdentity(tokenBytes)
	if err != nil {
		return workerRequest{}, "", "", fmt.Errorf("create offline validation token: %w", err)
	}
	nonce, err := randomIdentity(nonceBytes)
	if err != nil {
		return workerRequest{}, "", "", fmt.Errorf("create offline validation nonce: %w", err)
	}
	originRoots := make(map[string]string, len(acquisition.OriginRoots))
	for id, path := range acquisition.OriginRoots {
		originRoots[id] = path
	}
	request := workerRequest{
		Protocol: protocolVersion, Token: token, Nonce: nonce,
		ProjectRoot: acquisition.ProjectRoot, OriginRoots: originRoots,
	}
	request.RequestSHA256 = requestDigest(request)
	data, err := json.Marshal(request)
	if err != nil {
		return workerRequest{}, "", "", err
	}
	if int64(len(data)) > maxRequestFileBytes {
		return workerRequest{}, "", "", fmt.Errorf("offline validation request exceeds %d bytes", maxRequestFileBytes)
	}
	requestPath := filepath.Join(root, "request.json")
	responsePath := filepath.Join(root, "response.json")
	file, err := os.OpenFile(requestPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return workerRequest{}, "", "", fmt.Errorf("create offline validation request: %w", err)
	}
	wrote := false
	defer func() {
		_ = file.Close()
		if !wrote {
			_ = os.Remove(requestPath)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return workerRequest{}, "", "", fmt.Errorf("write offline validation request: %w", err)
	}
	if err := file.Close(); err != nil {
		return workerRequest{}, "", "", fmt.Errorf("close offline validation request: %w", err)
	}
	wrote = true
	return request, requestPath, responsePath, nil
}

func randomIdentity(size int) (string, error) {
	data := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func readWorkerResponse(path string, request workerRequest) (workerResponse, error) {
	data, err := readRegularFile(path, maxResponseFileBytes)
	if err != nil {
		return workerResponse{}, err
	}
	var response workerResponse
	if err := strictDecode(data, &response); err != nil {
		return workerResponse{}, err
	}
	if response.Protocol != protocolVersion || response.Token != request.Token || response.Nonce != request.Nonce || response.RequestSHA256 != request.RequestSHA256 {
		return workerResponse{}, errors.New("response identity does not match the current request")
	}
	if response.ResponseSHA256 != responseDigest(response) {
		return workerResponse{}, errors.New("response digest does not match its payload")
	}
	switch response.Status {
	case responseAccepted:
		if response.Gate != "" || response.Reason != "" {
			return workerResponse{}, errors.New("accepted response contains a rejection")
		}
		if response.ValidationSHA256 == "" || response.ValidationSHA256 != validationDigest(response.Validation) {
			return workerResponse{}, errors.New("validation digest does not match its payload")
		}
	case responseRejected:
		if !allowedValidationGate(response.Gate) || strings.TrimSpace(response.Reason) == "" {
			return workerResponse{}, errors.New("rejected response has an invalid gate or reason")
		}
		if response.ValidationSHA256 != "" || !reflect.DeepEqual(response.Validation, managedpack.Validation{}) {
			return workerResponse{}, errors.New("rejected response contains validation output")
		}
	default:
		return workerResponse{}, fmt.Errorf("unsupported response status %q", response.Status)
	}
	return response, nil
}

func allowedValidationGate(gate managedpackpromotion.Gate) bool {
	return gate == managedpackpromotion.GateValidation || gate == managedpackpromotion.GateOrigins ||
		gate == managedpackpromotion.GateExactCopies || gate == managedpackpromotion.GateNotices
}

func pathWithin(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

type processInvocation struct {
	Executable string
	Args       []string
	Env        []string
	Dir        string
	Stdout     io.Writer
	Stderr     io.Writer
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
	command, err := workerCommand(ctx, invocation)
	if err != nil {
		return err
	}
	command.Env = append([]string(nil), invocation.Env...)
	command.Dir = invocation.Dir
	command.Stdin = nil
	command.Stdout = invocation.Stdout
	command.Stderr = invocation.Stderr
	return command.Run()
}

type cappedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func newCappedBuffer(limit int) *cappedBuffer {
	return &cappedBuffer{limit: limit}
}

func (buffer *cappedBuffer) Write(data []byte) (int, error) {
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

func (buffer *cappedBuffer) Len() int         { return buffer.buffer.Len() }
func (buffer *cappedBuffer) String() string   { return buffer.buffer.String() }
func (buffer *cappedBuffer) Overflowed() bool { return buffer.overflow }

var _ managedpackpromotion.OfflineValidator = Adapter{}
