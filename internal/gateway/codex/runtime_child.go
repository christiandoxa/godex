package codex

import "context"

// RunRuntime applies the exact runtime-child argument policy before delegating
// to the low-level Codex process runner. Plain Run remains an argv-preserving
// primitive for non-runtime callers and tests.
func (process *CodexProcess) RunRuntime(ctx context.Context, codexHome string, arguments []string) error {
	if !codexCommandServerSubcommand(arguments) {
		arguments = codexTUIArguments(arguments)
		resetTerminalKeyboardEnhancementBestEffort(process.terminal.Stdout)
	}
	return process.run(ctx, codexHome, arguments)
}
