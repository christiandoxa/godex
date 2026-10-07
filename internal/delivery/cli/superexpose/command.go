package superexpose

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	bodyMaxBytes  = 1024 * 1024
	rateLimit     = 120
	rateWindow    = time.Second
	defaultListen = "127.0.0.1:0"
)

type Options struct {
	Mode           string
	Listen         string
	NoTunnel       bool
	Tunnel         bool
	OpenAITunnelID string
	Name           string
	DryRun         bool
	SuperArgs      []string
}

type rateLimiter struct {
	mu       sync.Mutex
	started  time.Time
	requests int
}

func (rate *rateLimiter) admit(now time.Time) bool {
	rate.mu.Lock()
	defer rate.mu.Unlock()
	if rate.started.IsZero() || now.Sub(rate.started) >= rateWindow {
		rate.started = now
		rate.requests = 0
	}
	if rate.requests >= rateLimit {
		return false
	}
	rate.requests++
	return true
}

func Run(ctx context.Context, arguments []string, out, errOut io.Writer) error {
	options, err := parseArguments(arguments)
	if err != nil {
		return err
	}
	if options.Tunnel {
		return errors.New("legacy --tunnel mode is not supported; use --openai-tunnel-id or omit tunnel flags for local-only access")
	}
	if options.OpenAITunnelID != "" {
		resolved, err := validateOpenAITunnelID(options.OpenAITunnelID)
		if err != nil {
			return err
		}
		options.OpenAITunnelID = resolved
	}
	address, err := net.ResolveTCPAddr("tcp", options.Listen)
	if err != nil {
		return fmt.Errorf("invalid expose listen address %s: %w", options.Listen, err)
	}
	if address.IP == nil || !address.IP.IsLoopback() {
		return errors.New("Godex Super expose only binds loopback addresses")
	}
	if options.DryRun {
		fmt.Fprintln(out, "Godex Super expose dry run")
		fmt.Fprintf(out, "Mode: %s\n", options.Mode)
		fmt.Fprintf(out, "Listen: %s\n", options.Listen)
		if options.OpenAITunnelID != "" {
			fmt.Fprintf(out, "Tunnel: OpenAI Secure MCP Tunnel %s\n", options.OpenAITunnelID)
		} else {
			fmt.Fprintln(out, "Tunnel: disabled (local only)")
		}
		fmt.Fprintln(out, "Status: dry run")
		return nil
	}
	return runExecServer(ctx, options, out, errOut)
}

func parseArguments(arguments []string) (Options, error) {
	options := Options{Mode: "full", Listen: defaultListen}
	modeSeen := false
	for index := 0; index < len(arguments); {
		argument := arguments[index]
		if argument == "--" {
			options.SuperArgs = append(options.SuperArgs, arguments[index:]...)
			break
		}
		if !modeSeen && (argument == "full" || argument == "exec") {
			options.Mode = argument
			modeSeen = true
			index++
			continue
		}
		switch argument {
		case "--no-tunnel":
			options.NoTunnel = true
			index++
			continue
		case "--tunnel":
			options.Tunnel = true
			index++
			continue
		case "--dry-run":
			options.DryRun = true
			index++
			continue
		}
		if value, consumed, ok, err := optionValue(arguments, index, "--listen"); ok {
			if err != nil {
				return Options{}, err
			}
			options.Listen = value
			index += consumed
			continue
		}
		if value, consumed, ok, err := optionValue(arguments, index, "--openai-tunnel-id"); ok {
			if err != nil {
				return Options{}, err
			}
			options.OpenAITunnelID = value
			index += consumed
			continue
		}
		if value, consumed, ok, err := optionValue(arguments, index, "--name"); ok {
			if err != nil {
				return Options{}, err
			}
			options.Name = value
			index += consumed
			continue
		}
		if superExposeEmbeddedOptionTakesValue(argument) && !strings.Contains(argument, "=") {
			if index+1 >= len(arguments) || arguments[index+1] == "--" {
				return Options{}, fmt.Errorf("%s requires a value", argument)
			}
			options.SuperArgs = append(options.SuperArgs, argument, arguments[index+1])
			index += 2
			continue
		}
		options.SuperArgs = append(options.SuperArgs, argument)
		index++
	}
	if options.NoTunnel && (options.Tunnel || options.OpenAITunnelID != "") {
		return Options{}, errors.New("--no-tunnel conflicts with tunnel options")
	}
	if options.Tunnel && options.OpenAITunnelID != "" {
		return Options{}, errors.New("--tunnel conflicts with --openai-tunnel-id")
	}
	return options, nil
}

func superExposeEmbeddedOptionTakesValue(argument string) bool {
	name := argument
	if before, _, ok := strings.Cut(argument, "="); ok {
		name = before
	}
	switch name {
	case "--profile", "-p", "--provider", "--cli", "--api-key",
		"--base-url", "--url", "--model", "--local-model",
		"--context-window", "--local-context-window",
		"--auto-compact-token-limit", "--local-auto-compact-token-limit",
		"--tool", "--require-tool",
		"--sub-agent-provider", "--sub-agent-model", "--sub-agent-model-reasoning-effort",
		"--sub-agent-url", "--sub-agent-max-concurrency",
		"--web-search", "--rollout-budget-tokens", "--rollout-budget-reminders",
		"--rollout-budget-sampling-weight", "--rollout-budget-prefill-weight",
		"--current-time-reminder-interval", "--current-time-clock-source",
		"-c", "--config":
		return true
	default:
		return false
	}
}

func optionValue(arguments []string, index int, name string) (string, int, bool, error) {
	argument := arguments[index]
	if argument == name {
		if index+1 >= len(arguments) || strings.TrimSpace(arguments[index+1]) == "" {
			return "", 0, true, fmt.Errorf("%s requires a value", name)
		}
		return arguments[index+1], 2, true, nil
	}
	prefix := name + "="
	if strings.HasPrefix(argument, prefix) {
		value := strings.TrimPrefix(argument, prefix)
		if strings.TrimSpace(value) == "" {
			return "", 0, true, fmt.Errorf("%s requires a value", name)
		}
		return value, 1, true, nil
	}
	return "", 0, false, nil
}

func runExecServerWithTunnelStarter(ctx context.Context, options Options, out, errOut io.Writer, startTunnel openAITunnelStarter) error {
	workspace, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve expose workspace: %w", err)
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return fmt.Errorf("failed to canonicalize expose workspace: %w", err)
	}
	audit, err := newExposeAuditLog(ctx)
	if err != nil {
		return err
	}
	tools := discoverOptionalTools()
	if err := audit.event("super_expose_starting", map[string]string{
		"mode":                     options.Mode,
		"bind":                     "loopback",
		"optional_tools_available": fmt.Sprint(len(tools.availableIDs())),
	}); err != nil {
		return fmt.Errorf("failed to create expose audit log: %w", err)
	}
	token, err := capabilityToken()
	if err != nil {
		return err
	}
	instanceID, err := exposeInstanceID()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", options.Listen)
	if err != nil {
		return fmt.Errorf("failed to bind Godex Super expose: %w", err)
	}
	defer listener.Close()
	_, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return errors.New("Godex Super expose did not bind an IP address")
	}
	displayName := strings.TrimSpace(options.Name)
	if displayName == "" {
		displayName = filepath.Base(workspace)
	}
	if displayName == "" || displayName == "." || displayName == string(filepath.Separator) {
		displayName = "workspace"
	}
	expectedPath := "/mcp/" + token
	endpoint := "http://" + listener.Addr().String() + expectedPath
	var runs *runManager
	var sessions *existingSessionService
	if options.Mode == "full" {
		runs, err = newRunManagerWithAudit(workspace, options.SuperArgs, instanceID, displayName, audit)
		if err != nil {
			return err
		}
		defer runs.shutdown()
		sessions = newExistingSessionService(systemSessionProcessInspector{}, systemSessionQueueControl{})
	}
	handler := &execMCPHandler{
		expectedPath:  expectedPath,
		expectedHost:  listener.Addr().String(),
		displayName:   displayName,
		instanceID:    instanceID,
		workspace:     workspace,
		mode:          options.Mode,
		runs:          runs,
		sessions:      sessions,
		optionalTools: tools,
		audit:         audit,
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	if options.OpenAITunnelID != "" {
		audit.tryEvent("super_expose_openai_tunnel_starting", map[string]string{
			"mode": options.Mode, "provider": "openai",
		})
	}
	fmt.Fprintf(out, "Godex Super expose %s\n", options.Mode)
	fmt.Fprintf(out, "Workspace: %s\n", displayName)
	fmt.Fprintf(out, "Mode: %s\n", options.Mode)
	fmt.Fprintf(out, "Endpoint: %s\n", endpoint)
	fmt.Fprintln(out, "Safety: capability URL is secret; local loopback only.")
	fmt.Fprintln(out, "Stop: Ctrl-C")
	if errOut != nil {
		fmt.Fprintln(errOut, "Godex Super Expose: capability URL is secret; Ctrl-C stops the endpoint.")
	}
	port := ""
	if tcpAddress, ok := listener.Addr().(*net.TCPAddr); ok {
		port = fmt.Sprint(tcpAddress.Port)
	}
	if err := audit.event("super_expose_started", map[string]string{
		"mode": options.Mode,
		"bind": "loopback",
		"port": port,
	}); err != nil {
		return fmt.Errorf("failed to flush expose audit log: %w", err)
	}

	serveErr := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveErr <- err
	}()

	var tunnel *openAITunnelProcess
	var tunnelReady <-chan error
	var tunnelExit <-chan error
	if options.OpenAITunnelID != "" {
		tunnel, err = startTunnel(endpoint, options.OpenAITunnelID)
		if err != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = server.Shutdown(closeCtx)
			cancel()
			<-serveErr
			return err
		}
		defer tunnel.shutdown()
		ready := make(chan error, 1)
		go func() {
			ready <- tunnel.waitReady(ctx)
			close(ready)
		}()
		tunnelReady = ready
		if errOut != nil {
			fmt.Fprintf(errOut, "Godex Super Expose: OpenAI Secure MCP Tunnel %s starting.\n", options.OpenAITunnelID)
		}
	}

	for {
		select {
		case err := <-serveErr:
			return err
		case err := <-tunnelReady:
			if err != nil {
				audit.tryEvent("super_expose_openai_tunnel_failed", map[string]string{
					"provider": "openai",
					"reason":   "startup_failed",
				})
				closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = server.Shutdown(closeCtx)
				cancel()
				<-serveErr
				return err
			}
			tunnelReady = nil
			if tunnel != nil {
				tunnelExit = tunnel.exit
				if err := audit.event("super_expose_openai_tunnel_ready", map[string]string{
					"provider":       "openai",
					"client_version": tunnel.version,
				}); err != nil {
					closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_ = server.Shutdown(closeCtx)
					cancel()
					<-serveErr
					return fmt.Errorf("failed to flush expose tunnel audit log: %w", err)
				}
				if errOut != nil {
					fmt.Fprintf(errOut, "Godex Super Expose: OpenAI Secure MCP Tunnel %s ready (client %s).\n", tunnel.id, tunnel.version)
				}
			}
		case exitErr := <-tunnelExit:
			audit.tryEvent("super_expose_openai_tunnel_exited", map[string]string{
				"provider":  "openai",
				"exit_code": tunnelExitCodeLabel(exitErr),
			})
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = server.Shutdown(closeCtx)
			cancel()
			<-serveErr
			return errors.New("OpenAI tunnel-client exited unexpectedly")
		case <-ctx.Done():
			_ = server.Close()
			<-serveErr
			return nil
		}
	}
}

func capabilityToken() (string, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate expose capability: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func exposeInstanceID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate expose instance id: %w", err)
	}
	return "gdxi_" + base64.RawURLEncoding.EncodeToString(bytes), nil
}
