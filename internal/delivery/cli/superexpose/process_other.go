//go:build !unix && !windows

package superexpose

import "os/exec"

func configureExecProcess(*exec.Cmd) {}

func stopExecProcessTree(command *exec.Cmd) {
	if command != nil && command.Process != nil {
		_ = command.Process.Kill()
	}
}

func execExitSignal(*exec.ExitError) (any, int) { return nil, 1 }
