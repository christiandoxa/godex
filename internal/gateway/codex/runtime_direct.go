package codex

import (
	"context"
	"strings"
)

var upstreamProxyEnvironmentKeys = map[string]bool{
	"HTTP_PROXY": true, "HTTPS_PROXY": true, "ALL_PROXY": true,
	"http_proxy": true, "https_proxy": true, "all_proxy": true,
	"PROXY": true, "proxy": true,
}

// RunRuntimeDirect starts Codex without the Godex runtime proxy while retaining
// the runtime child hardening/argument projection used by normal launches.
func (process *CodexProcess) RunRuntimeDirect(
	ctx context.Context,
	codexHome string,
	arguments []string,
	noProxy bool,
) error {
	if !codexCommandServerSubcommand(arguments) {
		arguments = codexTUIArguments(arguments)
		resetTerminalKeyboardEnhancementBestEffort(process.terminal.Stdout)
	}
	binary, err := process.resolveBinary()
	if err != nil {
		return err
	}
	if err := secureCodexHomeWithShared(codexHome, process.sharedCodexHome); err != nil {
		return err
	}
	release, err := (SessionLocker{}).LockCodexSessionsForChild(ctx, codexHome)
	if err != nil {
		return err
	}
	defer release()

	command := terminalCommand(ctx, binary, arguments)
	environment := codexThreadIndexEnvironment(codexHome, process.sharedCodexHome)
	if noProxy {
		environment = removeUpstreamProxyEnvironment(environment)
	}
	command.Env = environment
	command.Stdin = process.terminal.Stdin
	command.Stdout = process.terminal.Stdout
	command.Stderr = process.terminal.Stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return nil
}

func removeUpstreamProxyEnvironment(environment []string) []string {
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, found := strings.Cut(entry, "=")
		if found && upstreamProxyEnvironmentKeys[key] {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}
