package superexpose

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

func spawnOpenAITunnel(localMCPURL, tunnelID, binary, version string) (*openAITunnelProcess, error) {
	tunnelID, err := validateOpenAITunnelID(tunnelID)
	if err != nil {
		return nil, err
	}
	apiKey, err := openAITunnelAPIKeyFromEnv()
	if err != nil {
		return nil, err
	}
	localMCPURL, err = validateLocalTunnelMCPURL(localMCPURL)
	if err != nil {
		return nil, err
	}
	files, err := createOpenAITunnelFiles(localMCPURL)
	if err != nil {
		return nil, err
	}
	command := exec.Command(binary,
		"run", "--config", filepath.Join(files, "config.yaml"),
		"--control-plane.base-url", "https://api.openai.com",
		"--control-plane.tunnel-id", tunnelID,
		"--control-plane.api-key", "env:CONTROL_PLANE_API_KEY",
		"--health.listen-addr", "127.0.0.1:0",
		"--health.url-file", filepath.Join(files, "health-url"),
		"--log.level", "warn",
		"--log.format", "struct-text",
	)
	environment := inheritedEnvironment()
	removeInheritedTunnelConfiguration(environment)
	environment["CONTROL_PLANE_API_KEY"] = apiKey
	command.Env = flattenEnvironment(environment)
	command.Stdin = nil
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	configureExecProcess(command)
	if err := command.Start(); err != nil {
		_ = os.RemoveAll(files)
		return nil, fmt.Errorf("failed to spawn tunnel-client: %w", err)
	}
	exitCh := make(chan error, 1)
	go func() {
		exitCh <- command.Wait()
		close(exitCh)
	}()
	return &openAITunnelProcess{
		command: command, id: tunnelID, version: version,
		directory: files, healthURL: filepath.Join(files, "health-url"), exit: exitCh,
	}, nil
}
