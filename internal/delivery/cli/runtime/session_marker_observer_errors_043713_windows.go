//go:build windows

package runtime

import (
	"errors"
	"syscall"
)

// ERROR_TOO_MANY_OPEN_FILES is 4 on Windows. Do not map access denied or an
// invalid marker to transient resource pressure.
func sessionMarkerObserverTransient043713(err error) bool {
	return errors.Is(err, syscall.Errno(4))
}
