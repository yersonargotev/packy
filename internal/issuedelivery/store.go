package issuedelivery

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
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

type lockedIssueStore struct {
	directory int
}

type activeRun struct {
	RunID    string `json:"run_id"`
	Revision string `json:"revision,omitempty"`
}

func (fileRunStore) withIssueLock(
	ctx context.Context,
	commonDir string,
	issue int,
	fn func(lockedIssueStore) error,
) error {
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
	return fn(lockedIssueStore{directory: issueFD})
}

func (store lockedIssueStore) loadActive() (runID string, data []byte, found bool, err error) {
	active, found, err := store.loadActiveRecord()
	if err != nil || !found {
		return "", nil, found, err
	}
	if active.Revision != "" {
		revisionData, found, err := store.loadRevision(active.RunID, active.Revision)
		if err != nil {
			return "", nil, false, fmt.Errorf("load active issue delivery revision: %w", err)
		}
		if !found {
			return "", nil, false, errors.New("active issue delivery revision does not exist")
		}
		return active.RunID, revisionData, true, nil
	}
	runData, found, err := store.loadRun(active.RunID)
	if err != nil {
		return "", nil, false, fmt.Errorf("load active issue delivery run %q: %w", active.RunID, err)
	}
	if !found {
		return "", nil, false, errors.New("active issue delivery run does not exist")
	}
	return active.RunID, runData, true, nil
}

func (store lockedIssueStore) loadActiveRecord() (activeRun, bool, error) {
	activeData, err := readFileAt(store.directory, "active.json")
	if errors.Is(err, unix.ENOENT) {
		return activeRun{}, false, nil
	}
	if err != nil {
		return activeRun{}, false, fmt.Errorf("load active issue delivery run: %w", err)
	}
	active, err := decodeActive(activeData)
	if err != nil {
		return activeRun{}, false, err
	}
	return active, true, nil
}

func (store lockedIssueStore) loadRevision(runID, revision string) ([]byte, bool, error) {
	if !validRunID(runID) {
		return nil, false, errors.New("issue delivery run ID must be 64 lowercase hexadecimal characters")
	}
	if !validRunID(revision) {
		return nil, false, errors.New("issue delivery revision ID must be 64 lowercase hexadecimal characters")
	}
	revisionsFD, err := openDirectoryAt(store.directory, "revisions", false)
	if errors.Is(err, unix.ENOENT) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("inspect issue delivery revisions directory: %w", err)
	}
	defer unix.Close(revisionsFD)
	runFD, err := openDirectoryAt(revisionsFD, runID, false)
	if errors.Is(err, unix.ENOENT) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("inspect issue delivery run revisions: %w", err)
	}
	defer unix.Close(runFD)
	data, err := readFileAt(runFD, revision+".json")
	if errors.Is(err, unix.ENOENT) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load issue delivery revision: %w", err)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != revision {
		return nil, false, errors.New("issue delivery revision content does not match its identity")
	}
	return data, true, nil
}

func (store lockedIssueStore) loadRun(runID string) ([]byte, bool, error) {
	if !validRunID(runID) {
		return nil, false, errors.New("issue delivery run ID must be 64 lowercase hexadecimal characters")
	}
	runsFD, err := openDirectoryAt(store.directory, "runs", false)
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

func (store lockedIssueStore) activate(runID string) error {
	return store.activateRevision(runID, "")
}

func (store lockedIssueStore) activateRevision(runID, revision string) error {
	if !validRunID(runID) {
		return errors.New("issue delivery run ID must be 64 lowercase hexadecimal characters")
	}
	if revision != "" && !validRunID(revision) {
		return errors.New("issue delivery revision ID must be 64 lowercase hexadecimal characters")
	}
	activeData, err := json.Marshal(activeRun{RunID: runID, Revision: revision})
	if err != nil {
		return err
	}
	if err := atomicWriteAt(store.directory, "active.json", activeData); err != nil {
		return fmt.Errorf("activate issue delivery run: %w", err)
	}
	return nil
}

func (store lockedIssueStore) storeRevisionAndActivate(runID string, data []byte) (string, error) {
	if !validRunID(runID) {
		return "", errors.New("issue delivery run ID must be 64 lowercase hexadecimal characters")
	}
	if _, found, err := store.loadRun(runID); err != nil {
		return "", fmt.Errorf("inspect issue delivery run: %w", err)
	} else if !found {
		return "", errors.New("issue delivery run does not exist")
	}

	digest := sha256.Sum256(data)
	revision := hex.EncodeToString(digest[:])
	revisionsFD, err := openDirectoryAt(store.directory, "revisions", true)
	if err != nil {
		return "", err
	}
	defer unix.Close(revisionsFD)
	runFD, err := openDirectoryAt(revisionsFD, runID, true)
	if err != nil {
		return "", err
	}
	defer unix.Close(runFD)

	revisionName := revision + ".json"
	if existing, err := readFileAt(runFD, revisionName); err == nil {
		if !bytes.Equal(existing, data) {
			return "", errors.New("issue delivery revision is immutable")
		}
	} else if !errors.Is(err, unix.ENOENT) {
		return "", fmt.Errorf("inspect issue delivery revision: %w", err)
	} else if err := atomicWriteAt(runFD, revisionName, data); err != nil {
		return "", fmt.Errorf("store issue delivery revision: %w", err)
	}
	if err := store.activateRevision(runID, revision); err != nil {
		return "", err
	}
	return revision, nil
}

func (store lockedIssueStore) storeAndActivate(runID string, data []byte) error {
	if !validRunID(runID) {
		return errors.New("issue delivery run ID must be 64 lowercase hexadecimal characters")
	}
	runsFD, err := openDirectoryAt(store.directory, "runs", true)
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
	return store.activate(runID)
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
	if !bytes.Equal(data, canonical) ||
		!validRunID(active.RunID) ||
		(active.Revision != "" && !validRunID(active.Revision)) {
		return activeRun{}, errors.New("active issue delivery run is not canonical")
	}
	return active, nil
}

func validRunID(runID string) bool {
	return runIDPattern.MatchString(runID)
}
