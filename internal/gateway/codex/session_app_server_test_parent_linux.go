//go:build linux

package codex

import "syscall"

func clearSessionCompanionParentDeath() {
	_, _, _ = syscall.Syscall6(syscall.SYS_PRCTL, syscall.PR_SET_PDEATHSIG, 0, 0, 0, 0, 0)
}
