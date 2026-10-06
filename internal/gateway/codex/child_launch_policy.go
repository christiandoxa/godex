package codex

import (
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

const (
	godexAllowUnsafeChildEnv             = "GODEX_ALLOW_UNSAFE_CHILD_ENV"
	codexDisableKeyboardEnhancementEnv   = "CODEX_TUI_DISABLE_KEYBOARD_ENHANCEMENT"
	codexDisablePasteBurstConfigKey      = "disable_paste_burst"
	codexDisablePasteBurstConfigOverride = "disable_paste_burst=true"
)

var dangerousChildEnvKeys = map[string]bool{
	"LD_PRELOAD": true, "LD_AUDIT": true, "LD_LIBRARY_PATH": true, "LD_ORIGIN_PATH": true,
	"DYLD_INSERT_LIBRARIES": true, "DYLD_LIBRARY_PATH": true, "DYLD_FRAMEWORK_PATH": true,
}

var codexCommandServerSubcommands = map[string]bool{
	"mcp-server":  true,
	"app-server":  true,
	"exec-server": true,
}

func hardenCodexChildEnvironment(environment []string) []string {
	allowUnsafe := childEnvironmentHasKey(environment, godexAllowUnsafeChildEnv)
	parts := make([]string, 0, 8)
	result := make([]string, 0, len(environment)+3)
	for _, entry := range environment {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			result = append(result, entry)
			continue
		}
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "CODEX_SANDBOX") {
			continue
		}
		if !allowUnsafe && (dangerousChildEnvKeys[upper] || strings.HasPrefix(upper, "DYLD_")) {
			continue
		}
		if strings.EqualFold(key, codexDisableKeyboardEnhancementEnv) {
			continue
		}
		if key == "NO_PROXY" || key == "no_proxy" {
			appendNoProxyParts(&parts, value)
			continue
		}
		result = append(result, entry)
	}
	for _, value := range []string{"127.0.0.1", "localhost", "::1"} {
		appendNoProxyPart(&parts, value)
	}
	merged := strings.Join(parts, ",")
	result = append(result,
		"NO_PROXY="+merged,
		"no_proxy="+merged,
		codexDisableKeyboardEnhancementEnv+"=1",
	)
	return result
}

func childEnvironmentHasKey(environment []string, target string) bool {
	for _, entry := range environment {
		key, _, found := strings.Cut(entry, "=")
		if found && strings.EqualFold(key, target) {
			return true
		}
	}
	return false
}

func appendNoProxyParts(parts *[]string, value string) {
	for _, part := range strings.Split(value, ",") {
		appendNoProxyPart(parts, part)
	}
}

func appendNoProxyPart(parts *[]string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, current := range *parts {
		if strings.EqualFold(current, value) {
			return
		}
	}
	*parts = append(*parts, value)
}

func codexCommandServerSubcommand(arguments []string) bool {
	if len(arguments) == 0 {
		return false
	}
	return codexCommandServerSubcommands[arguments[0]]
}

func codexTUIArguments(arguments []string) []string {
	result := append([]string(nil), arguments...)
	if codexExecInvocation(result) || codexConfigOverridePresent(result, codexDisablePasteBurstConfigKey) {
		return result
	}
	return append([]string{"-c", codexDisablePasteBurstConfigOverride}, result...)
}

func codexExecInvocation(arguments []string) bool {
	command := commandAfterOptions(arguments, 0)
	return command >= 0 && (arguments[command] == "exec" || arguments[command] == "e")
}

func codexConfigOverridePresent(arguments []string, target string) bool {
	for index := 0; index < len(arguments); {
		if arguments[index] == "--" {
			return false
		}
		value, consumed, ok := configArgument(arguments, index)
		if !ok {
			index += consumed
			continue
		}
		key, _, found := strings.Cut(strings.TrimSpace(value), "=")
		if found && normalizeConfigKey(key) == target {
			return true
		}
		index += consumed
	}
	return false
}

func normalizeConfigKey(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "\"") {
		if decoded, err := strconv.Unquote(value); err == nil {
			return decoded
		}
	}
	return strings.Trim(value, "'")
}

const terminalKeyboardEnhancementPop = "\x1b[<1u"

func resetTerminalKeyboardEnhancementBestEffort(writer io.Writer) {
	file, ok := writer.(*os.File)
	if !ok || file == nil || !term.IsTerminal(int(file.Fd())) {
		return
	}
	_, _ = io.WriteString(writer, terminalKeyboardEnhancementPop)
}
