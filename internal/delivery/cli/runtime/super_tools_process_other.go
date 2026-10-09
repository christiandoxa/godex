//go:build !unix && !windows

package runtime

import "os/exec"

func configureSuperProbeProcess(*exec.Cmd) {}

func stopSuperProbeProcessTree(command *exec.Cmd) {
	if command != nil && command.Process != nil {
		_ = command.Process.Kill()
	}
}
