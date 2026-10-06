//go:build unix

package runtime

import "os"

func createSuperOverlaySymlink(target, link string, _ bool) error {
	return os.Symlink(target, link)
}
