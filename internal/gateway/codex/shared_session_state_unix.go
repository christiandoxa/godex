//go:build !windows

package codex

import "os"

func createSharedStateSymlink(target, link string, _ bool) error {
	return os.Symlink(target, link)
}
