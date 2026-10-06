//go:build !unix && !windows

package subagent

import "os/exec"

func configureProcessGroup(*exec.Cmd) {}

func stopProcessTree(command *exec.Cmd) {
	if command != nil && command.Process != nil {
		_ = command.Process.Kill()
	}
}

func signalExitCode(*exec.ExitError) int { return 1 }
