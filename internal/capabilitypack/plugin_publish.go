package capabilitypack

import (
	"context"
	"crypto/rand"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// publishPlugin anchors all output mutations to one open parent directory.
// Rename publishes the complete staged tree in one operation; an existing empty
// directory is never removed in advance. A nonempty destination cannot be replaced.
func publishPlugin(ctx context.Context, snapshotRoot, output string, files map[string]pluginFile) (string, error) {
	if output == "" {
		return "", fmt.Errorf("export requires a caller-selected --output directory")
	}
	destination, err := filepath.Abs(output)
	if err != nil {
		return "", err
	}
	destination = filepath.Clean(destination)
	parentPath, err := filepath.EvalSymlinks(filepath.Dir(destination))
	if err != nil {
		return "", fmt.Errorf("export output parent must already exist: %w", err)
	}
	destination = filepath.Join(parentPath, filepath.Base(destination))
	source, err := filepath.EvalSymlinks(snapshotRoot)
	if err != nil {
		return "", err
	}
	if pluginPathContains(source, destination) || pluginPathContains(destination, source) || destination == filepath.VolumeName(destination)+string(filepath.Separator) {
		return "", fmt.Errorf("export output must be separate from the retained Catalog Snapshot")
	}
	parent, err := os.OpenRoot(parentPath)
	if err != nil {
		return "", err
	}
	defer parent.Close()
	name := filepath.Base(destination)
	before, err := inspectPluginDestination(parent, name)
	if err != nil {
		return "", err
	}
	stageName := ".packy-export-" + rand.Text()
	if err := parent.Mkdir(stageName, 0700); err != nil {
		return "", err
	}
	defer parent.RemoveAll(stageName)
	stage, err := parent.OpenRoot(stageName)
	if err != nil {
		return "", err
	}
	defer stage.Close()
	for relative, file := range files {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if err := stage.MkdirAll(path.Dir(relative), 0755); err != nil {
			return "", err
		}
		// Chmod regular open handles so ambient umask never changes required modes.
		handle, err := stage.OpenFile(relative, os.O_CREATE|os.O_EXCL|os.O_WRONLY, file.mode)
		if err != nil {
			return "", err
		}
		_, writeErr := handle.Write(file.data)
		if writeErr == nil {
			writeErr = handle.Chmod(file.mode)
		}
		closeErr := handle.Close()
		if writeErr != nil {
			return "", writeErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	// Normalize directory modes too, then verify the complete actual staged tree.
	err = fs.WalkDir(stage.FS(), ".", func(relative string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return stage.Chmod(relative, 0755)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if err := verifyStagedPlugin(stage, files); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	after, err := inspectPluginDestination(parent, name)
	if err != nil {
		return "", err
	}
	if (before == nil) != (after == nil) || before != nil && !os.SameFile(before, after) {
		return "", fmt.Errorf("export output changed during staging")
	}
	// os.Root.Rename rejects every existing directory, including empty ones.
	// POSIX renameat provides the required atomic empty-directory replacement.
	directory, err := parent.Open(".")
	if err != nil {
		return "", err
	}
	defer directory.Close()
	if err := unix.Renameat(int(directory.Fd()), stageName, int(directory.Fd()), name); err != nil {
		return "", fmt.Errorf("publish complete plugin: %w", err)
	}
	return destination, nil
}

func inspectPluginDestination(parent *os.Root, name string) (fs.FileInfo, error) {
	info, err := parent.Lstat(name)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("export output must be a new or empty regular directory")
	}
	directory, err := parent.Open(name)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	if len(entries) != 0 {
		return nil, fmt.Errorf("export output directory is not empty")
	}
	return info, nil
}

func verifyStagedPlugin(root *os.Root, files map[string]pluginFile) error {
	observed := map[string]pluginFile{}
	err := fs.WalkDir(root.FS(), ".", func(relative string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := root.Lstat(relative)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected staged plugin path %s", relative)
		}
		data, err := root.ReadFile(relative)
		if err != nil {
			return err
		}
		observed[relative] = pluginFile{data, info.Mode().Perm()}
		return nil
	})
	if err != nil {
		return err
	}
	if len(files) != len(observed) || pluginFilesDigest(files) != pluginFilesDigest(observed) {
		return fmt.Errorf("staged plugin content or modes differ from validated package")
	}
	return nil
}

func pluginPathContains(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
