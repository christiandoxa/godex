//go:build !aix && !darwin && !dragonfly && !freebsd && !hurd && !illumos && !linux && !netbsd && !openbsd && !solaris

package processresource

// PrepareDescriptorHeadroom is a no-op on platforms without RLIMIT_NOFILE.
func PrepareDescriptorHeadroom() error { return nil }
