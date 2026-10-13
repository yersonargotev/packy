package catalogorigin

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// copyTree hashes an ordered, length-delimited description of the complete tree.
// An empty target only seals; otherwise the exact bytes hashed are also copied.
// Symlinks are copied as links and never followed, including inside Git metadata.
func copyTree(ctx context.Context, source, target string) (string, error) {
	if err := realDirectory(source); err != nil {
		return "", err
	}
	digest := sha256.New()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		fmt.Fprintf(digest, "%d:%s:%d:", len(name), name, info.Mode())
		destination := ""
		if target != "" {
			destination = filepath.Join(target, relative)
		}
		switch {
		case info.IsDir():
			if destination != "" {
				if err := os.Mkdir(destination, info.Mode().Perm()); err != nil {
					return err
				}
				return os.Chmod(destination, info.Mode().Perm())
			}
		case info.Mode().IsRegular():
			fmt.Fprintf(digest, "%d:", info.Size())
			input, err := os.Open(path)
			if err != nil {
				return err
			}
			defer input.Close()
			var output *os.File
			var writer io.Writer = digest
			if destination != "" {
				output, err = os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
				if err != nil {
					return err
				}
				if err := output.Chmod(info.Mode().Perm()); err != nil {
					_ = output.Close()
					return err
				}
				writer = io.MultiWriter(digest, output)
			}
			n, err := io.Copy(writer, contextReader{ctx, input})
			if output != nil {
				closeErr := output.Close()
				if err == nil {
					err = closeErr
				}
			}
			if err != nil {
				return err
			}
			if n != info.Size() {
				return fmt.Errorf("origin file %s changed while reading", name)
			}
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(digest, "%d:%s", len(link), link)
			if destination != "" {
				return os.Symlink(link, destination)
			}
		default:
			return fmt.Errorf("unsupported origin file type: %s", name)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

type contextReader struct {
	ctx   context.Context
	input io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.input.Read(p)
}
