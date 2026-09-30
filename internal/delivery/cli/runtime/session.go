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
	start := -1
	if command >= 0 {
		switch args[command] {
		case "resume", "fork", "delete", "archive", "unarchive":
			start = command + 1
		case "queue":
			return queueArgument(args, command+1), args
		}
	}
	if start < 0 {
		return -1, args
	}
	options := true
	for i := start; i < len(args); i++ {
		if options {
			if args[i] == "--" {
				options = false
				continue
			}
			if args[i] == "--last" {
				return -1, args
			}
			if nativeOptionTakesValue(args[i]) {
				i++
				continue
			}
			if strings.HasPrefix(args[i], "-") {
				continue
			}
		}
		if !explicitSessionSelector(args[i]) {
			return -1, args
		}
		return i, args
	}
	return -1, args
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
	}
	return false
}
