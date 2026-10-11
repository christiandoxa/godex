//go:build aix || darwin || dragonfly || freebsd || hurd || illumos || linux || netbsd || openbsd || solaris

package processresource

import "golang.org/x/sys/unix"

const descriptorSoftTarget = 8_192

// PrepareDescriptorHeadroom raises only the inherited soft descriptor limit.
func PrepareDescriptorHeadroom() error {
	var limits unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &limits); err != nil {
		return err
	}
	target := min(uint64(descriptorSoftTarget), limits.Max)
	if limits.Cur >= target {
		return nil
	}
	limits.Cur = target
	return unix.Setrlimit(unix.RLIMIT_NOFILE, &limits)
}
