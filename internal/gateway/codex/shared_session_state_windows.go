//go:build windows

package codex

import "os"

func createSharedStateSymlink(target, link string, directory bool) error {
	if directory {
		return os.Symlink(target, link)
	}
	return os.Symlink(target, link)
}
