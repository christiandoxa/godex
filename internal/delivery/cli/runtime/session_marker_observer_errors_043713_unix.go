//go:build !windows

package runtime

import (
	"errors"
	"syscall"
)

// Only typed temporary OS resource errors defer the observer. Malformed
// identities and permanent permission failures must not become retryable.
func sessionMarkerObserverTransient043713(err error) bool {
	return errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE) ||
		errors.Is(err, syscall.EINTR) || errors.Is(err, syscall.EAGAIN)
}
