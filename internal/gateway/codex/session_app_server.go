package codex

import (
	"strconv"
	"strings"
)

// sessionAppServerEligible limits the private companion to interactive
// foreground launches. Exec/resume and an already remote child own their
// session transport and must keep their native argument shape.
func sessionAppServerEligible(arguments []string) bool {
	if len(arguments) == 0 || codexCommandServerSubcommand(arguments) || codexExecInvocation(arguments) {
		return false
	}
	command := commandAfterOptions(arguments, 0)
	if command >= 0 && (arguments[command] == "resume" || arguments[command] == "app-server") {
		return false
	}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			break
		}
		if argument == "--remote" || strings.HasPrefix(argument, "--remote=") {
			return false
		}
		if _, consumed, ok := configArgument(arguments, index); ok {
			index += consumed - 1
			continue
		}
		if codexOptionTakesValue(argument) && index+1 < len(arguments) {
			index++
		}
	}
	return true
}

func sessionAppServerCompanionArguments(arguments []string, socket string) []string {
	result := []string{"app-server", "--listen", "unix://" + socket}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			break
		}
		switch argument {
		case "-c", "--config", "--enable", "--disable", "--code-mode-host":
			if index+1 < len(arguments) {
				result = append(result, argument, arguments[index+1])
				index++
			}
		case "--strict-config":
			result = append(result, argument)
		case "-m", "--model":
			if index+1 < len(arguments) {
				result = append(result, "-c", "model="+strconv.Quote(arguments[index+1]))
				index++
			}
		default:
			if strings.HasPrefix(argument, "--model=") {
				result = append(result, "-c", "model="+strconv.Quote(strings.TrimPrefix(argument, "--model=")))
			}
		}
	}
	return result
}
