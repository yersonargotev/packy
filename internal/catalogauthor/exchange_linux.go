//go:build linux

package catalogauthor

import "golang.org/x/sys/unix"

func exchangePaths(left, right string) error {
	return unix.Renameat2(unix.AT_FDCWD, left, unix.AT_FDCWD, right, unix.RENAME_EXCHANGE)
}
