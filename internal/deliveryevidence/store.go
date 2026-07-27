package deliveryevidence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type atomicFile interface {
	Name() string
	Chmod(os.FileMode) error
	Write([]byte) (int, error)
	Sync() error
	Close() error
}
type atomicDirectory interface {
	Sync() error
	Close() error
}
type atomicOps struct {
	MkdirAll      func(string, os.FileMode) error
	CreateTemp    func(string, string) (atomicFile, error)
	Rename        func(string, string) error
	OpenDirectory func(string) (atomicDirectory, error)
	Remove        func(string) error
}

func defaultAtomicOps() atomicOps {
	return atomicOps{os.MkdirAll, func(d, p string) (atomicFile, error) { return os.CreateTemp(d, p) }, os.Rename, func(p string) (atomicDirectory, error) { return os.Open(p) }, os.Remove}
}

// ResolvePath uses Git's common directory so linked worktrees share authority.
func ResolvePath(commonDir, override string, issue int) (string, error) {
	if issue <= 0 {
		return "", errors.New("issue number must be positive")
	}
	if override != "" {
		if !filepath.IsAbs(override) {
			return "", errors.New("delivery evidence override must be absolute")
		}
		return filepath.Clean(override), nil
	}
	if commonDir == "" || !filepath.IsAbs(commonDir) {
		return "", errors.New("Git common directory must be absolute")
	}
	return filepath.Join(filepath.Clean(commonDir), "packy", "issue-delivery", fmt.Sprintf("issue-%d.json", issue)), nil
}

func Load(path string) (Bundle, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Bundle{}, nil, err
	}
	b, err := Decode(data)
	return b, data, err
}

func StoreAtomic(path string, bundle Bundle) error {
	return storeAtomicWithOps(path, bundle, defaultAtomicOps())
}
func storeAtomicWithOps(path string, bundle Bundle, ops atomicOps) (retErr error) {
	data, err := CanonicalJSON(bundle)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err = ops.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := ops.CreateTemp(dir, ".issue-delivery-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			first := ops.Remove(tmp)
			if first != nil {
				second := ops.Remove(tmp)
				retErr = errors.Join(retErr, fmt.Errorf("remove temporary delivery evidence: %w", first))
				if second != nil {
					retErr = errors.Join(retErr, fmt.Errorf("retry remove temporary delivery evidence: %w", second))
				}
			}
		}
	}()
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err = ops.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	d, e := ops.OpenDirectory(dir)
	if e != nil {
		return e
	}
	if e = d.Sync(); e != nil {
		return errors.Join(e, d.Close())
	}
	if e = d.Close(); e != nil {
		return e
	}
	return nil
}

type ResumeState string

const (
	Initialized ResumeState = "initialized"
	Resumed     ResumeState = "resumed"
	Stale       ResumeState = "stale"
)

type ResumeResult struct {
	State  ResumeState
	Bundle Bundle
	Path   string
}

// InitializeOrResume creates absent evidence, resumes only exact authority, and
// never overwrites stale evidence.
func InitializeOrResume(path string, qualified Bundle) (ResumeResult, error) {
	want, err := CanonicalJSON(qualified)
	if err != nil {
		return ResumeResult{}, err
	}
	existing, _, err := Load(path)
	if errors.Is(err, os.ErrNotExist) {
		if err = StoreAtomic(path, qualified); err != nil {
			return ResumeResult{}, err
		}
		return ResumeResult{Initialized, qualified, path}, nil
	}
	if err != nil {
		return ResumeResult{}, err
	}
	got, _ := CanonicalJSON(existing)
	if string(got) == string(want) {
		return ResumeResult{Resumed, existing, path}, nil
	}
	return ResumeResult{Stale, existing, path}, nil
}
