package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
)

const (
	geminiRestartResponseCase04384 = "raw-key-bound-response-restart"
	geminiRestartKeyCase04384      = "raw-key-bound-response-restart-key"
	geminiRestartEndpointCase04384 = "raw-key-bound-response-restart-endpoint"
)

func geminiRestartCase04384(scenario string) bool {
	return scenario == geminiRestartResponseCase04384 || scenario == geminiRestartKeyCase04384 || scenario == geminiRestartEndpointCase04384
}

func geminiRestartArguments04384(scenario string, arguments []string) []string {
	arguments = slices.Clone(arguments)
	for index := 0; index+1 < len(arguments); index++ {
		if scenario == geminiRestartKeyCase04384 && arguments[index] == "--api-key" {
			arguments[index+1] = rotationSecondaryKey
		}
		if scenario == geminiRestartEndpointCase04384 && arguments[index] == "--base-url" {
			arguments[index+1] += "/different"
		}
	}
	return arguments
}

func runGeminiRestartSeed04384(ctx context.Context, binary string, arguments, env []string, home, childResult string, mock *geminiMock) (*geminiCandidateResult, error) {
	stateHome := filepath.Join(home, "state")
	cmd := exec.CommandContext(ctx, binary, arguments...)
	cmd.Env = append(slices.Clone(env), "DIFFERENTIAL_CONTINUATION_PHASE=seed", "DIFFERENTIAL_STATE_HOME="+stateHome)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	exitCode := 0
	if err := cmd.Run(); err != nil {
		var childExit *exec.ExitError
		if !errors.As(err, &childExit) || ctx.Err() != nil {
			return nil, fmt.Errorf("Gemini restart seed failed: %w", err)
		}
		exitCode = childExit.ExitCode()
	}
	client, err := readExchange(childResult)
	if err != nil {
		return nil, fmt.Errorf("Gemini restart seed exchange missing: %w stderr=%q", err, redact(stderr.String()))
	}
	seed := &geminiCandidateResult{ExitCode: exitCode, Client: client, Stdout: stdout.String(), Upstream: mock.recorded(),
		ContinuationPersisted: rawKeyProbeBindingExists(stateHome, filepath.Base(home) == "prodex", "chatcmpl-differential")}
	owners := geminiContinuationOwners04384(geminiRestartResponseCase04384, *seed, mock.server.URL+"/v1beta")
	seed.StateIntegrity = auditFixtureDurableStateWithBindings(home, owners)
	return seed, nil
}
