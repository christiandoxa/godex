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
	scope := 0
	for i := 0; i < len(arguments); {
		argument := arguments[i]
		if argument == "--" {
			break
		}
		if _, consumed, ok := configArgument(arguments, i); ok {
			i += consumed
			continue
		}
		if strings.HasPrefix(argument, "-") {
			if codexOptionTakesValue(argument) {
				i += 2
			} else {
				i++
			}
			continue
		}
		if scope == 0 && (argument == "exec" || argument == "e") {
			scope = i + 1
			i++
			continue
		}
		if scope > 0 && (argument == "resume" || argument == "fork" || argument == "review") {
			scope = i + 1
		}
		break
	}
	return scope
}

func codexOptionTakesValue(argument string) bool {
	switch argument {
	case "--model", "-m", "--profile", "-p", "--cd", "-C", "--sandbox", "-s", "--ask-for-approval", "-a", "--image", "-i", "--enable", "--disable", "--color", "--output-last-message", "-o", "--output-schema", "--local-provider", "--add-dir", "--thread-source":
		return true
	}
	return false
}
