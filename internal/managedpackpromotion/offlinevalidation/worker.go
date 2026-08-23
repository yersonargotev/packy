package offlinevalidation

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yersonargotev/packy/internal/managedpack"
	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

const (
	maxRequestFileBytes   = int64(1 << 20)
	maxResponseFileBytes  = int64(16 << 20)
	maxProcessOutputBytes = 64 << 10
)

var originIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Run executes one offline validation worker request. The caller owns process
// isolation and passes the request and response paths after routing the hidden
// worker mode.
func Run(args []string, stdout, stderr io.Writer) int {
	return runWithBoundary(args, stdout, stderr, installWorkerBoundary)
}

func runWithBoundary(args []string, stdout, stderr io.Writer, install func() error) int {
	if install == nil {
		fmt.Fprintln(stderr, "install offline validation boundary: installer is required")
		return 1
	}
	if err := install(); err != nil {
		fmt.Fprintf(stderr, "install offline validation boundary: %v\n", err)
		return 1
	}
	if len(args) != 2 {
		fmt.Fprintln(stderr, "offline validation worker requires request and response paths")
		return 2
	}
	request, err := readWorkerRequest(args[0], args[1])
	if err != nil {
		fmt.Fprintf(stderr, "read offline validation request: %v\n", err)
		return 1
	}

	preflight, validationErr := managedpack.Preflight(context.Background(), request.ProjectRoot, mapResolver(request.OriginRoots))
	response := workerResponse{
		Protocol: request.Protocol, Token: request.Token, Nonce: request.Nonce,
		RequestSHA256: request.RequestSHA256,
	}
	if validationErr != nil {
		response.Status = responseRejected
		response.Gate = validationGate(validationErr)
		response.Reason = validationErr.Error()
	} else {
		validation := preflight.Validation
		response.Status = responseAccepted
		response.Validation = validation
		response.ValidationSHA256 = validationDigest(validation)
	}
	response.ResponseSHA256 = responseDigest(response)
	if err := writeWorkerResponse(args[1], response); err != nil {
		fmt.Fprintf(stderr, "write offline validation response: %v\n", err)
		return 1
	}
	return 0
}

type mapResolver map[string]string

func (resolver mapResolver) Resolve(ctx context.Context, origin managedpack.Origin) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	root, ok := resolver[origin.ID]
	if !ok {
		return "", fmt.Errorf("origin %q was not acquired", origin.ID)
	}
	return root, nil
}

func readWorkerRequest(requestPath, responsePath string) (workerRequest, error) {
	if !filepath.IsAbs(requestPath) || !filepath.IsAbs(responsePath) {
		return workerRequest{}, errors.New("protocol paths must be absolute local paths")
	}
	if filepath.Clean(requestPath) == filepath.Clean(responsePath) {
		return workerRequest{}, errors.New("request and response paths must be distinct")
	}
	data, err := readRegularFile(requestPath, maxRequestFileBytes)
	if err != nil {
		return workerRequest{}, err
	}
	var request workerRequest
	if err := strictDecode(data, &request); err != nil {
		return workerRequest{}, err
	}
	if request.Protocol != protocolVersion {
		return workerRequest{}, fmt.Errorf("unsupported protocol version %d", request.Protocol)
	}
	if !validRandomIdentity(request.Token, tokenBytes) || !validRandomIdentity(request.Nonce, nonceBytes) {
		return workerRequest{}, errors.New("request token or nonce is invalid")
	}
	if request.RequestSHA256 != requestDigest(request) {
		return workerRequest{}, errors.New("request digest does not match its payload")
	}
	if err := validateAcquisitionRoots(request.ProjectRoot, request.OriginRoots); err != nil {
		return workerRequest{}, err
	}
	return request, nil
}

func validateAcquisitionRoots(projectRoot string, originRoots map[string]string) error {
	if !filepath.IsAbs(projectRoot) {
		return errors.New("Managed Pack Project root must be an absolute local path")
	}
	if originRoots == nil {
		return errors.New("origin roots must be a non-null map")
	}
	acquisitionRoot := filepath.Dir(filepath.Clean(projectRoot))
	if filepath.Base(filepath.Clean(projectRoot)) != "project" {
		return errors.New("Managed Pack Project root must be the acquired project directory")
	}
	if err := requireRealDirectory(acquisitionRoot); err != nil {
		return fmt.Errorf("inspect acquisition root: %w", err)
	}
	if err := requireRealDirectoryUnder(acquisitionRoot, projectRoot); err != nil {
		return fmt.Errorf("inspect Managed Pack Project root: %w", err)
	}
	for id, root := range originRoots {
		if !originIDPattern.MatchString(id) {
			return fmt.Errorf("origin root id %q must be lowercase kebab-case", id)
		}
		if !filepath.IsAbs(root) || filepath.Clean(root) != filepath.Join(acquisitionRoot, "origins", id) {
			return fmt.Errorf("origin %q root must be its acquired local directory", id)
		}
		if err := requireRealDirectoryUnder(acquisitionRoot, root); err != nil {
			return fmt.Errorf("inspect origin %q root: %w", id, err)
		}
	}
	return nil
}

func requireRealDirectoryUnder(root, target string) error {
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("directory is outside its acquisition root")
	}
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		if err := requireRealDirectory(current); err != nil {
			return fmt.Errorf("component %q: %w", component, err)
		}
	}
	return nil
}

func requireRealDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("must be a directory and not a symlink")
	}
	return nil
}

func readRegularFile(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("protocol input must be a regular file and not a symlink")
	}
	if info.Size() < 0 || info.Size() > maximum {
		return nil, fmt.Errorf("protocol input exceeds %d bytes", maximum)
	}
	return os.ReadFile(path)
}

func writeWorkerResponse(path string, response workerResponse) error {
	parent := filepath.Dir(path)
	if err := requireRealDirectory(parent); err != nil {
		return fmt.Errorf("inspect response directory: %w", err)
	}
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}
	if int64(len(data)) > maxResponseFileBytes {
		return fmt.Errorf("protocol output exceeds %d bytes", maxResponseFileBytes)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	written := false
	defer func() {
		_ = file.Close()
		if !written {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	written = true
	return nil
}

func strictDecode(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func validRandomIdentity(value string, size int) bool {
	if len(value) != size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == size
}

func validationGate(err error) managedpackpromotion.Gate {
	if managedpack.IsRuntimeFitnessFailure(err) {
		return managedpackpromotion.GateResourceSurfaces
	}
	if managedpack.IsExactCopyMismatch(err) {
		return managedpackpromotion.GateExactCopies
	}
	reason := strings.ToLower(err.Error())
	switch {
	case strings.Contains(reason, "notice"):
		return managedpackpromotion.GateNotices
	case strings.Contains(reason, "origin"):
		return managedpackpromotion.GateOrigins
	default:
		return managedpackpromotion.GateValidation
	}
}
