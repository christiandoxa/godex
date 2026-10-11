//go:build !linux && !windows

package codex

import "syscall"

func configureCodexAppServerParentDeath(*syscall.SysProcAttr) {}
