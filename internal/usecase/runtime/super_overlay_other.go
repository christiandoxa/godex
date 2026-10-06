//go:build !unix && !windows

package runtime

import "errors"

func createSuperOverlaySymlink(string, string, bool) error {
	return errors.New("Godex Super overlay links are not supported on this platform")
}
