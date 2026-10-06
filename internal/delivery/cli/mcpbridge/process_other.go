//go:build !unix

package mcpbridge

import "os/exec"

func configureProcessGroup(*exec.Cmd) {}
func stopProcessTree(command *exec.Cmd) {
	if command != nil && command.Process != nil {
		_ = command.Process.Kill()
	}
}
