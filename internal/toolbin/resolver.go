// Package toolbin observes external tool requirements through the host PATH.
package toolbin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

type PATHResolver struct {
	lookPath func(string) (string, error)
}

func NewPATHResolver(lookPath func(string) (string, error)) PATHResolver {
	return PATHResolver{lookPath: lookPath}
}

func (r PATHResolver) Resolve(_ context.Context, tool string) (capabilitypack.ExecutableResolution, error) {
	resolution := capabilitypack.ExecutableResolution{
		Tool: tool, Origin: "path", Precondition: "missing|PATH|" + tool,
	}
	if r.lookPath == nil {
		return resolution, nil
	}
	path, err := r.lookPath(tool)
	if err != nil || path == "" {
		return resolution, nil
	}
	resolved := path
	if canonical, canonicalErr := filepath.EvalSymlinks(path); canonicalErr == nil {
		resolved = canonical
	}
	resolution.Available = true
	resolution.Path = path
	resolution.ResolvedPath = resolved
	resolution.Precondition = executablePrecondition(path, resolved)
	return resolution, nil
}

func executablePrecondition(path, resolved string) string {
	info, err := os.Stat(path)
	if err != nil {
		return path + "|" + resolved + "|unstatable:" + err.Error()
	}
	return fmt.Sprintf("%s|%s|%d|%d|%o", path, resolved, info.Size(), info.ModTime().UnixNano(), info.Mode().Perm())
}
