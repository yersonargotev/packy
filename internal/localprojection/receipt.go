package localprojection

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ValidateReceiptTarget checks an adapter-owned path before receipt inspection.
// Only skill projections may follow a terminal symlink; their contents must
// still consist entirely of ordinary files and directories.
func ValidateReceiptTarget(root, target string, allowSymlink bool) error {
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("receipt target %q is outside adapter root %q", target, root)
	}
	for parent := filepath.Dir(target); ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return fmt.Errorf("unsafe receipt target parent %q", parent)
		}
		if parent == filepath.Clean(root) {
			break
		}
		if parent == filepath.Dir(parent) {
			return fmt.Errorf("receipt target %q has no adapter root", target)
		}
	}
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !allowSymlink {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsafe receipt file target %q", target)
		}
		return nil
	}
	tree := target
	if info.Mode()&os.ModeSymlink != 0 {
		tree, err = filepath.EvalSymlinks(target)
		// A dangling skill link is observable drift, not permission to follow it.
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("unsafe receipt skill target %q: %w", target, err)
		}
	}
	return filepath.WalkDir(tree, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("unsafe receipt skill entry %q", path)
		}
		return nil
	})
}
