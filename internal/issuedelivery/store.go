package issuedelivery

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
	"regexp"

	"golang.org/x/sys/unix"
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

	issueFD, err := openIssueDirectory(commonDir, issue, true)
	if err != nil {
		return err
	}
	defer unix.Close(issueFD)
	lockFD, err := unix.Openat(
		issueFD, "advance.lock",
		unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0600,
	)
	if err != nil {
		return fmt.Errorf("open issue delivery lock: %w", err)
	}
	defer unix.Close(lockFD)
	if err := requireRegularFD(lockFD, "advance.lock"); err != nil {
		return err
	}
	if err := unix.Flock(lockFD, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return errIssueRunActive
		}
		return fmt.Errorf("lock issue delivery run: %w", err)
	}
	defer unix.Flock(lockFD, unix.LOCK_UN)

	if err := ctx.Err(); err != nil {
		return err
	}
	return fn()
}

func (fileRunStore) loadActive(commonDir string, issue int) (runID string, data []byte, found bool, err error) {
	issueFD, err := openIssueDirectory(commonDir, issue, false)
	if errors.Is(err, unix.ENOENT) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	defer unix.Close(issueFD)

	activeData, err := readFileAt(issueFD, "active.json")
	if errors.Is(err, unix.ENOENT) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, fmt.Errorf("load active issue delivery run: %w", err)
	}
	active, err := decodeActive(activeData)
	if err != nil {
		return "", nil, false, err
	}
	runsFD, err := openDirectoryAt(issueFD, "runs", false)
	if err != nil {
		return "", nil, false, fmt.Errorf("inspect issue delivery runs directory: %w", err)
	}
	defer unix.Close(runsFD)
	runData, err := readFileAt(runsFD, active.RunID+".json")
	if err != nil {
		return "", nil, false, fmt.Errorf("load issue delivery run %q: %w", active.RunID, err)
	}
	return active.RunID, runData, true, nil
}

func (fileRunStore) loadRun(commonDir string, issue int, runID string) ([]byte, bool, error) {
	if !validRunID(runID) {
		return nil, false, errors.New("issue delivery run ID must be 64 lowercase hexadecimal characters")
	}
	issueFD, err := openIssueDirectory(commonDir, issue, false)
	if errors.Is(err, unix.ENOENT) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer unix.Close(issueFD)
	runsFD, err := openDirectoryAt(issueFD, "runs", false)
	if errors.Is(err, unix.ENOENT) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer unix.Close(runsFD)
	data, err := readFileAt(runsFD, runID+".json")
	if errors.Is(err, unix.ENOENT) {
		return nil, false, nil
	}
	return data, err == nil, err
}

func (fileRunStore) activate(commonDir string, issue int, runID string) error {
	if !validRunID(runID) {
		return errors.New("issue delivery run ID must be 64 lowercase hexadecimal characters")
	}
	issueFD, err := openIssueDirectory(commonDir, issue, true)
	if err != nil {
		return err
	}
	defer unix.Close(issueFD)
	activeData, err := json.Marshal(activeRun{RunID: runID})
	if err != nil {
		return err
	}
	if err := atomicWriteAt(issueFD, "active.json", activeData); err != nil {
		return fmt.Errorf("activate issue delivery run: %w", err)
	}
	return nil
}

func (store fileRunStore) storeAndActivate(commonDir string, issue int, runID string, data []byte) error {
	if !validRunID(runID) {
		return errors.New("issue delivery run ID must be 64 lowercase hexadecimal characters")
	}
	issueFD, err := openIssueDirectory(commonDir, issue, true)
	if err != nil {
		return err
	}
	defer unix.Close(issueFD)
	runsFD, err := openDirectoryAt(issueFD, "runs", true)
	if err != nil {
		return err
	}
	defer unix.Close(runsFD)

	runName := runID + ".json"
	if existing, err := readFileAt(runsFD, runName); err == nil {
		if !bytes.Equal(existing, data) {
			return errors.New("issue delivery run is immutable")
		}
	} else if !errors.Is(err, unix.ENOENT) {
		return fmt.Errorf("inspect issue delivery run: %w", err)
	} else if err := atomicWriteAt(runsFD, runName, data); err != nil {
		return fmt.Errorf("store issue delivery run: %w", err)
	}
	return store.activate(commonDir, issue, runID)
}

func openIssueDirectory(commonDir string, issue int, create bool) (int, error) {
	if issue <= 0 {
		return -1, errors.New("issue number must be positive")
	}
	if commonDir == "" || !filepath.IsAbs(commonDir) || filepath.Clean(commonDir) != commonDir {
		return -1, errors.New("Git common directory must be an absolute canonical path")
	}
	current, err := unix.Open(commonDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, fmt.Errorf("open Git common directory: %w", err)
	}
	for _, name := range []string{"packy", "issue-delivery", fmt.Sprintf("issue-%d", issue)} {
		next, openErr := openDirectoryAt(current, name, create)
		unix.Close(current)
		if openErr != nil {
			return -1, openErr
		}
		current = next
	}
	return current, nil
}

func openDirectoryAt(parent int, name string, create bool) (int, error) {
	flags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	fd, err := unix.Openat(parent, name, flags, 0)
	if errors.Is(err, unix.ENOENT) && create {
		if mkdirErr := unix.Mkdirat(parent, name, 0700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
			return -1, fmt.Errorf("create issue delivery directory %q: %w", name, mkdirErr)
		}
		fd, err = unix.Openat(parent, name, flags, 0)
	}
	if err != nil {
		return -1, fmt.Errorf("open issue delivery directory %q: %w", name, err)
	}
	if create {
		if err := unix.Fchmod(fd, 0700); err != nil {
			unix.Close(fd)
			return -1, fmt.Errorf("secure issue delivery directory %q: %w", name, err)
		}
	}
	return fd, nil
}

func readFileAt(directory int, name string) ([]byte, error) {
	fd, err := unix.Openat(directory, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	if err := requireRegularFD(fd, name); err != nil {
		return nil, err
	}
	return io.ReadAll(file)
}

func atomicWriteAt(directory int, name string, data []byte) (retErr error) {
	if err := rejectNonRegularAt(directory, name); err != nil {
		return err
	}
	tempName, tempFD, err := createTempAt(directory)
	if err != nil {
		return err
	}
	temp := os.NewFile(uintptr(tempFD), tempName)
	defer func() {
		if temp != nil {
			retErr = errors.Join(retErr, temp.Close())
		}
		if tempName != "" {
			if err := unix.Unlinkat(directory, tempName, 0); err != nil && !errors.Is(err, unix.ENOENT) {
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
	if err := unix.Renameat(directory, tempName, directory, name); err != nil {
		return err
	}
	tempName = ""
	return unix.Fsync(directory)
}

func createTempAt(directory int) (string, int, error) {
	for attempts := 0; attempts < 100; attempts++ {
		var random [12]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", -1, err
		}
		name := ".tmp-" + hex.EncodeToString(random[:])
		fd, err := unix.Openat(
			directory, name,
			unix.O_CREAT|unix.O_EXCL|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW,
			0600,
		)
		if errors.Is(err, unix.EEXIST) {
			continue
		}
		return name, fd, err
	}
	return "", -1, errors.New("create issue delivery temporary file: exhausted unique names")
}

func rejectNonRegularAt(directory int, name string) error {
	var stat unix.Stat_t
	err := unix.Fstatat(directory, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return fmt.Errorf("issue delivery entry %q is not a regular file", name)
	}
	return nil
}

func requireRegularFD(fd int, name string) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return fmt.Errorf("issue delivery entry %q is not a regular file", name)
	}
	return nil
}

func decodeActive(data []byte) (activeRun, error) {
	var active activeRun
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&active); err != nil {
		return activeRun{}, fmt.Errorf("decode active issue delivery run: %w", err)
	}
	canonical, err := json.Marshal(active)
	if err != nil {
		return activeRun{}, err
	}
	if !bytes.Equal(data, canonical) || !validRunID(active.RunID) {
		return activeRun{}, errors.New("active issue delivery run is not canonical")
	}
	return active, nil
}

func validRunID(runID string) bool {
	return runIDPattern.MatchString(runID)
}
