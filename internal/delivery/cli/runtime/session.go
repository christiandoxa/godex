package runtime

import (
	"strings"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
)

// Only explicit UUIDs/prefixes are cross-profile. Native names and pickers stay local.
func sessionArgument(arguments []string) (int, []string) {
	args := append([]string(nil), arguments...)
	if len(args) == 1 && sessionentity.ValidID(args[0]) {
		return 1, []string{"resume", args[0]}
	}
	command := nativeCommandIndex(args)
	if command < 0 {
		return -1, args
	}
	if explicitSessionSelector(args[command]) {
		rewritten := make([]string, 0, len(args)+1)
		rewritten = append(rewritten, args[:command]...)
		rewritten = append(rewritten, "resume", args[command])
		rewritten = append(rewritten, args[command+1:]...)
		return command + 1, rewritten
	}
	switch args[command] {
	case "queue":
		return queueArgument(args, command+1), args
	case "resume":
		return findResumeSessionSelector(args, command+1), args
	case "fork":
		return findForkSessionSelector(args, command+1), args
	case "delete", "archive", "unarchive":
		return findExplicitSessionSelector(args, command+1), args
	default:
		return -1, args
	}
}

func findResumeSessionSelector(args []string, start int) int {
	for i := start; i < len(args); {
		if args[i] == "--" {
			if i+1 < len(args) && strings.TrimSpace(args[i+1]) != "" {
				return i + 1
			}
			return -1
		}
		if args[i] == "--last" {
			return i
		}
		if nativeOptionTakesValue(args[i]) {
			i += 2
			continue
		}
		if strings.HasPrefix(args[i], "-") {
			i++
			continue
		}
		return i
	}
	return -1
}

func findForkSessionSelector(args []string, start int) int {
	for i := start; i < len(args); {
		if args[i] == "--" {
			return selectorAfterDelimiter(args, i+1)
		}
		if args[i] == "--last" {
			return i
		}
		if nativeOptionTakesValue(args[i]) {
			i += 2
			continue
		}
		if strings.HasPrefix(args[i], "-") {
			i++
			continue
		}
		if explicitSessionSelector(args[i]) {
			return i
		}
		return -1
	}
	return -1
}

func findExplicitSessionSelector(args []string, start int) int {
	for i := start; i < len(args); {
		if args[i] == "--" {
			return selectorAfterDelimiter(args, i+1)
		}
		if args[i] == "--last" {
			return -1
		}
		if nativeOptionTakesValue(args[i]) {
			i += 2
			continue
		}
		if strings.HasPrefix(args[i], "-") {
			i++
			continue
		}
		if explicitSessionSelector(args[i]) {
			return i
		}
		return -1
	}
	return -1
}

func selectorAfterDelimiter(args []string, index int) int {
	if index < len(args) && explicitSessionSelector(args[index]) {
		return index
	}
	return -1
}

func explicitSessionSelector(value string) bool {
	if len(value) < 4 {
		return false
	}
	for _, r := range strings.ToLower(value) {
		if !strings.ContainsRune("0123456789abcdef-", r) {
			return false
		}
	}
	return true
}

func queueArgument(args []string, start int) int {
	for i := start; i < len(args); i++ {
		if args[i] == "--" {
			break
		}
		if args[i] == "--thread" && i+1 < len(args) && explicitSessionSelector(args[i+1]) {
			return i + 1
		}
		if value, ok := strings.CutPrefix(args[i], "--thread="); ok && explicitSessionSelector(value) {
			return i
		}
		if args[i] == "--message" || nativeOptionTakesValue(args[i]) {
			i++
		}
	}
	return -1
}

// Root option values and prompt text must not be mistaken for native commands.
func nativeCommandIndex(arguments []string) int {
	index := nextCommandWord(arguments, 0)
	if index < 0 || (arguments[index] != "exec" && arguments[index] != "e") {
		return index
	}
	nested := nextCommandWord(arguments, index+1)
	if nested >= 0 {
		switch arguments[nested] {
		case "resume", "fork", "review":
			return nested
		}
	}
	return index
}

func codexResumeRequested(arguments []string) bool {
	command := nativeCommandIndex(arguments)
	return command >= 0 && arguments[command] == "resume"
}

func nextCommandWord(arguments []string, start int) int {
	for i := start; i < len(arguments); i++ {
		if arguments[i] == "--" {
			return -1
		}
		if !strings.HasPrefix(arguments[i], "-") {
			return i
		}
		if arguments[i] == "--version" {
			return i
		}
		if nativeOptionTakesValue(arguments[i]) {
			i++
		}
	}
	return -1
}

func nativeOptionTakesValue(argument string) bool {
	switch argument {
	case "-c", "--config", "-m", "--model", "-C", "--cd", "-i", "--image", "-p", "--profile", "-s", "--sandbox", "-a", "--ask-for-approval", "--enable", "--disable", "--add-dir", "--color", "-o", "--output-last-message", "--output-schema", "--thread-source", "--local-provider", "--listen", "--code-mode-host":
		return true
	case "--remote", "--remote-auth-token-env", "--ws-auth", "--ws-token-file", "--ws-token-sha256", "--ws-shared-secret-file", "--ws-issuer", "--ws-audience", "--ws-max-clock-skew-seconds":
		return true
	}
	return false
}
