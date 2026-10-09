package runtime

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

const superProbeReapTimeout = time.Second

func superProbeCommand(program string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), superToolProbeTimeout)
	defer cancel()
	return runSuperProbeCommand(ctx, program, args...)
}

func runSuperProbeCommand(ctx context.Context, program string, args ...string) (string, bool) {
	if err := ctx.Err(); err != nil {
		return "", false
	}
	command := exec.Command(program, args...)
	configureSuperProbeProcess(command)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		return "", false
	}
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()

	select {
	case err := <-wait:
		if err != nil || ctx.Err() != nil {
			return "", false
		}
		return output.String(), true
	case <-ctx.Done():
		stopSuperProbeProcessTree(command)
		select {
		case <-wait:
		case <-time.After(superProbeReapTimeout):
		}
		return "", false
	}
}
