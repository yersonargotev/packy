//go:build darwin

package catalogauthor

import "golang.org/x/sys/unix"

func exchangePaths(left, right string) error {
	return unix.RenamexNp(left, right, unix.RENAME_SWAP)
}
