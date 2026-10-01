package codex

import (
	"strconv"
	"strings"
)

func configArgument(arguments []string, index int) (string, int, bool) {
	argument := arguments[index]
	if argument == "-c" || argument == "--config" {
		if index+1 < len(arguments) {
			return arguments[index+1], 2, true
		}
		return "", 1, true
	}
	if strings.HasPrefix(argument, "--config=") {
		return strings.TrimPrefix(argument, "--config="), 1, true
	}
	if strings.HasPrefix(argument, "-c") && argument != "-C" {
		return strings.TrimPrefix(strings.TrimPrefix(argument, "-c"), "="), 1, true
	}
	return "", 1, false
}

func ownedConfigKey(value string) bool {
	key, _, _ := strings.Cut(strings.TrimSpace(value), "=")
	for _, part := range strings.Split(key, ".") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, `"`) {
			if decoded, err := strconv.Unquote(part); err == nil {
				part = decoded
			}
		}
		part = strings.Trim(part, "'")
		switch part {
		case "chatgpt_base_url", "openai_base_url", "model_provider", "model_providers", "cli_auth_credentials_store":
			return true
		}
	}
	return false
}

// Codex exec/resume parse their own config overrides; preserve ordering within that scope.
func scopeModelArguments(arguments, managed []string) []string {
	scope := execConfigScope(arguments)
	if scope == 0 {
		return append(append([]string(nil), managed...), arguments...)
	}
	before, overrides := make([]string, 0, scope), make([]string, 0, scope)
	for i := 0; i < scope; {
		_, consumed, isConfig := configArgument(arguments, i)
		if isConfig {
			overrides = append(overrides, arguments[i:i+consumed]...)
		} else {
			before = append(before, arguments[i:i+consumed]...)
		}
		i += consumed
	}
	result := append(before, managed...)
	result = append(result, overrides...)
	return append(result, arguments[scope:]...)
}

func execConfigScope(arguments []string) int {
	command := commandAfterOptions(arguments, 0)
	if command < 0 || (arguments[command] != "exec" && arguments[command] != "e") {
		return 0
	}
	nested := commandAfterOptions(arguments, command+1)
	if nested >= 0 && nestedExecCommand(arguments[nested]) {
		return nested + 1
	}
	return command + 1
}

func commandAfterOptions(arguments []string, start int) int {
	for i := start; i < len(arguments); {
		if arguments[i] == "--" {
			return -1
		}
		if _, consumed, ok := configArgument(arguments, i); ok {
			i += consumed
			continue
		}
		if !strings.HasPrefix(arguments[i], "-") {
			return i
		}
		if codexOptionTakesValue(arguments[i]) {
			i += 2
		} else {
			i++
		}
	}
	return -1
}

func nestedExecCommand(command string) bool {
	return command == "resume" || command == "fork" || command == "review"
}

func codexOptionTakesValue(argument string) bool {
	switch argument {
	case "--model", "-m", "--profile", "-p", "--cd", "-C", "--sandbox", "-s", "--ask-for-approval", "-a", "--image", "-i", "--enable", "--disable", "--color", "--output-last-message", "-o", "--output-schema", "--local-provider", "--add-dir", "--thread-source":
		return true
	}
	return false
}
