//go:build linux

package bootstrap

import "golang.org/x/sys/unix"

func renameInstalledSourceNoReplace(source, destination string) error {
	return unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE)
}
