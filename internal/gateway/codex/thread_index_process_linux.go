//go:build linux

package codex

import "syscall"

// Prodex 0.437.3 gives its session-owned companion a parent-death guard. The
// process-group cleanup handles normal teardown; this closes the orphan window
// when Godex itself is terminated before it can run that cleanup.
func configureCodexAppServerParentDeath(attributes *syscall.SysProcAttr) {
	attributes.Pdeathsig = syscall.SIGTERM
}
