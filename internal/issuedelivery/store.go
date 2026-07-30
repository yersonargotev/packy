package issuedelivery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
)

var errIssueRunActive = errors.New("issue delivery run is active")

var runIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type fileRunStore struct{}

type activeRun struct {
	RunID string `json:"run_id"`
}

func (fileRunStore) withIssueLock(ctx context.Context, commonDir string, issue int, fn func() error) error {
	if ctx == nil {
		return errors.New("issue delivery lock requires a context")
	}
	if fn == nil {
		return errors.New("issue delivery lock requires an operation")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	issueDir, err := ensureIssueDirectory(commonDir, issue)
	if err != nil {
		return err
	}
	lockPath := filepath.Join(issueDir, "advance.lock")
	if err := rejectSymlinkIfPresent(lockPath); err != nil {
		return err
	}
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("open issue delivery lock: %w", err)
	}
	defer lock.Close()
	if err := validateRegularFile(lockPath, lock); err != nil {
		return err
	}

	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return errIssueRunActive
		}
		return fmt.Errorf("lock issue delivery run: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	if err := ctx.Err(); err != nil {
		return err
	}
	return fn()
}

func (fileRunStore) loadActive(commonDir string, issue int) (runID string, data []byte, found bool, err error) {
	issueDir, err := issueDirectory(commonDir, issue)
	if err != nil {
		return "", nil, false, err
	}
	if err := validateIssueDirectoryChain(commonDir, issueDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil, false, nil
		}
		return "", nil, false, err
	}

	activePath := filepath.Join(issueDir, "active.json")
	activeData, err := readRegularFile(activePath)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, fmt.Errorf("load active issue delivery run: %w", err)
	}

	var active activeRun
	decoder := json.NewDecoder(bytes.NewReader(activeData))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&active); err != nil {
		return "", nil, false, fmt.Errorf("decode active issue delivery run: %w", err)
	}
	canonical, err := json.Marshal(active)
	if err != nil {
		return "", nil, false, err
	}
	if !bytes.Equal(activeData, canonical) || !validRunID(active.RunID) {
		return "", nil, false, errors.New("active issue delivery run is not canonical")
	}

	runPath := filepath.Join(issueDir, "runs", active.RunID+".json")
	if err := validateExistingDirectory(filepath.Dir(runPath)); err != nil {
		return "", nil, false, fmt.Errorf("inspect issue delivery runs directory: %w", err)
	}
	runData, err := readRegularFile(runPath)
	if err != nil {
		return "", nil, false, fmt.Errorf("load issue delivery run %q: %w", active.RunID, err)
	}
	return active.RunID, runData, true, nil
}

func (fileRunStore) storeAndActivate(commonDir string, issue int, runID string, data []byte) error {
	if !validRunID(runID) {
		return errors.New("issue delivery run ID must be 64 lowercase hexadecimal characters")
	}
	issueDir, err := ensureIssueDirectory(commonDir, issue)
	if err != nil {
		return err
	}
	runsDir := filepath.Join(issueDir, "runs")
	if err := ensureDirectory(runsDir); err != nil {
		return err
	}

	runPath := filepath.Join(runsDir, runID+".json")
	if err := atomicWrite(runPath, data); err != nil {
		return fmt.Errorf("store issue delivery run: %w", err)
	}
	activeData, err := json.Marshal(activeRun{RunID: runID})
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(issueDir, "active.json"), activeData); err != nil {
		return fmt.Errorf("activate issue delivery run: %w", err)
	}
	return nil
}

func issueDirectory(commonDir string, issue int) (string, error) {
	if issue <= 0 {
		return "", errors.New("issue number must be positive")
	}
	if commonDir == "" || !filepath.IsAbs(commonDir) || filepath.Clean(commonDir) != commonDir {
		return "", errors.New("Git common directory must be an absolute canonical path")
	}
	info, err := os.Lstat(commonDir)
	if err != nil {
		return "", fmt.Errorf("inspect Git common directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("Git common directory must be a non-symlink directory")
	}
	return filepath.Join(commonDir, "packy", "issue-delivery", fmt.Sprintf("issue-%d", issue)), nil
}

func ensureIssueDirectory(commonDir string, issue int) (string, error) {
	issueDir, err := issueDirectory(commonDir, issue)
	if err != nil {
		return "", err
	}
	current := commonDir
	for _, name := range []string{"packy", "issue-delivery", filepath.Base(issueDir)} {
		current = filepath.Join(current, name)
		if err := ensureDirectory(current); err != nil {
			return "", err
		}
	}
	return issueDir, nil
}

func validateIssueDirectoryChain(commonDir, issueDir string) error {
	current := commonDir
	for _, name := range []string{"packy", "issue-delivery", filepath.Base(issueDir)} {
		current = filepath.Join(current, name)
		if err := validateExistingDirectory(current); err != nil {
			return err
		}
	}
	return nil
}

func ensureDirectory(path string) error {
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("create issue delivery directory %q: %w", path, err)
	}
	if err := validateExistingDirectory(path); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}

func validateExistingDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("issue delivery path %q is not a non-symlink directory", path)
	}
	return nil
}

func rejectSymlinkIfPresent(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("issue delivery path %q must not be a symlink", path)
	}
	return nil
}

func validateRegularFile(path string, file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect issue delivery file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("issue delivery path %q is not a regular file", path)
	}
	return nil
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("issue delivery path %q is not a regular non-symlink file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err := validateRegularFile(path, file); err != nil {
		return nil, err
	}
	return io.ReadAll(file)
}

func atomicWrite(path string, data []byte) (retErr error) {
	info, err := os.Lstat(path)
	if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return fmt.Errorf("issue delivery path %q is not a regular non-symlink file", path)
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	dir := filepath.Dir(path)
	if err := validateExistingDirectory(dir); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		if temp != nil {
			retErr = errors.Join(retErr, temp.Close())
		}
		if tempPath != "" {
			if err := os.Remove(tempPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				retErr = errors.Join(retErr, err)
			}
		}
	}()
	if err := temp.Chmod(0600); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	temp = nil
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	tempPath = ""
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		return errors.Join(err, directory.Close())
	}
	return directory.Close()
}

func validRunID(runID string) bool {
	return runIDPattern.MatchString(runID)
}
